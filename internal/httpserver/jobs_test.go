package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hoa-agent-backend/internal/jobs"
)

type fakeJobStore struct {
	job *jobs.Job
	err error
}

func (s *fakeJobStore) Get(ctx context.Context, id string) (*jobs.Job, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.job == nil || s.job.ID != id {
		return nil, jobs.ErrNotFound
	}
	return s.job, nil
}

func TestGetJob_Returns200WithJob(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	store := &fakeJobStore{job: &jobs.Job{
		ID:        "j1",
		Status:    jobs.StatusQueued,
		SkillName: "echo",
		InputJSON: json.RawMessage(`{"message":"hi"}`),
		CreatedAt: created,
		UpdatedAt: created,
	}}

	r := NewRouter(Options{Jobs: store})

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/j1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var got struct {
		Ok  bool     `json:"ok"`
		Job jobs.Job `json:"job"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Ok != true {
		t.Fatalf("expected ok=true")
	}
	if got.Job.ID != "j1" {
		t.Fatalf("expected job.id %q, got %q", "j1", got.Job.ID)
	}
	if got.Job.Status != jobs.StatusQueued {
		t.Fatalf("expected job.status %q, got %q", jobs.StatusQueued, got.Job.Status)
	}
	if got.Job.SkillName != "echo" {
		t.Fatalf("expected job.skill_name %q, got %q", "echo", got.Job.SkillName)
	}
}

func TestGetJob_Unknown_Returns404(t *testing.T) {
	r := NewRouter(Options{Jobs: &fakeJobStore{}})

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/does-not-exist", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", res.StatusCode)
	}
}
