package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"hoa-agent-backend/internal/jobs"
)

func openAsyncTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Ensure we always use the same connection for in-memory.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return db
}

func TestInvokeSkill_Async_ReturnsJobID(t *testing.T) {
	db := openAsyncTestDB(t)
	store := jobs.NewSQLiteStore(db)
	queue := make(chan string, 16)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartWorkerPool(ctx, 1, queue, store)

	r := NewRouter(Options{Jobs: store, Queue: queue})

	req := httptest.NewRequest(http.MethodPost, "/v1/skills/sleep_echo:invoke", strings.NewReader(`{"input": {"message": "hi"}, "trace": {"id": "t1"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 200, got %d (body=%q)", res.StatusCode, string(b))
	}

	var got struct {
		Ok    bool   `json:"ok"`
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Ok != true {
		t.Fatalf("expected ok=true")
	}
	if got.JobID == "" {
		t.Fatalf("expected non-empty job_id")
	}
}

func TestInvokeSkill_Async_JobEventuallySucceedsWithOutput(t *testing.T) {
	db := openAsyncTestDB(t)
	store := jobs.NewSQLiteStore(db)
	queue := make(chan string, 16)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartWorkerPool(ctx, 1, queue, store)

	r := NewRouter(Options{Jobs: store, Queue: queue})

	req := httptest.NewRequest(http.MethodPost, "/v1/skills/sleep_echo:invoke", strings.NewReader(`{"input": {"message": "hi", "sleep_ms": 1}, "trace": {"id": "t1"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 200, got %d (body=%q)", res.StatusCode, string(b))
	}

	var invokeRes struct {
		Ok    bool   `json:"ok"`
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&invokeRes); err != nil {
		t.Fatalf("decode invoke response: %v", err)
	}
	if invokeRes.JobID == "" {
		t.Fatalf("expected non-empty job_id")
	}

	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			t.Fatalf("timed out waiting for job to succeed")
		case <-ticker.C:
		}

		jobReq := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+invokeRes.JobID, nil)
		jobW := httptest.NewRecorder()
		r.ServeHTTP(jobW, jobReq)
		jobRes := jobW.Result()

		if jobRes.StatusCode != http.StatusOK {
			jobRes.Body.Close()
			t.Fatalf("expected status 200 for get job, got %d", jobRes.StatusCode)
		}

		var got struct {
			Ok  bool     `json:"ok"`
			Job jobs.Job `json:"job"`
		}
		if err := json.NewDecoder(jobRes.Body).Decode(&got); err != nil {
			jobRes.Body.Close()
			t.Fatalf("decode job response: %v", err)
		}
		jobRes.Body.Close()

		if got.Job.Status == jobs.StatusSucceeded {
			var out map[string]any
			if err := json.Unmarshal(got.Job.OutputJSON, &out); err != nil {
				t.Fatalf("unmarshal output_json: %v", err)
			}

			want := map[string]any{
				"input": map[string]any{"message": "hi", "sleep_ms": float64(1)},
				"trace": map[string]any{"id": "t1"},
			}
			if !jsonEqual(t, out, want) {
				gotB, _ := json.Marshal(out)
				wantB, _ := json.Marshal(want)
				t.Fatalf("unexpected output_json: got=%s want=%s", string(gotB), string(wantB))
			}
			return
		}

		if got.Job.Status == jobs.StatusFailed {
			t.Fatalf("job failed: %s", got.Job.Error)
		}
	}
}

func jsonEqual(t *testing.T, a any, b any) bool {
	t.Helper()
	aB, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal a: %v", err)
	}
	bB, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal b: %v", err)
	}
	return string(aB) == string(bB)
}
