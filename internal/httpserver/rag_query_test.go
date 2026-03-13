package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSkillsRoute_GET_IncludesRAGQuerySkill(t *testing.T) {
	r := NewRouter(Options{})

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 200, got %d (body=%q)", res.StatusCode, string(b))
	}

	var got struct {
		Skills []struct {
			Name    string `json:"name"`
			IsAsync bool   `json:"is_async"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var found *struct {
		Name    string
		IsAsync bool
	}
	for i := range got.Skills {
		if got.Skills[i].Name == "rag.query" {
			found = &struct {
				Name    string
				IsAsync bool
			}{Name: got.Skills[i].Name, IsAsync: got.Skills[i].IsAsync}
			break
		}
	}
	if found == nil {
		t.Fatalf("expected /v1/skills to include rag.query; got=%v", got.Skills)
	}
	if found.IsAsync != false {
		t.Fatalf("expected rag.query is_async=false, got %v", found.IsAsync)
	}
}

func TestInvokeSkill_RAGQuery_ReturnsOkWithHits(t *testing.T) {
	// Mock Qdrant search endpoint.
	var gotQdrantRequest struct {
		Vector []float64 `json:"vector"`
		Limit  int       `json:"limit"`
	}
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("qdrant: expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/collections/test_collection/points/search" {
			t.Fatalf("qdrant: unexpected path %q", r.URL.Path)
		}

		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("qdrant: decode request: %v", err)
		}

		// Capture a couple of fields for assertions.
		if v, ok := reqBody["vector"].([]any); ok {
			for _, x := range v {
				if f, ok := x.(float64); ok {
					gotQdrantRequest.Vector = append(gotQdrantRequest.Vector, f)
				}
			}
		}
		if lim, ok := reqBody["limit"].(float64); ok {
			gotQdrantRequest.Limit = int(lim)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"result": [
				{
					"id": "chunk_1",
					"score": 0.42,
					"payload": {
						"doc_id": "doc_1",
						"chunk_id": "chunk_1",
						"title": "Hello",
						"url": "https://example.invalid/hello",
						"snippet": "hello world",
						"source": "test"
					}
				}
			]
		}`))
	}))
	defer qdrant.Close()

	// These env vars are part of the contract we expect the implementation to use.
	t.Setenv("QDRANT_URL", qdrant.URL)
	t.Setenv("QDRANT_COLLECTION", "test_collection")

	// Ensure we never call a real external embedding API in tests.
	t.Setenv("EMBEDDING_PROVIDER", "stub")
	t.Setenv("EMBEDDING_STUB_VECTOR", "0.1,0.2,0.3")

	r := NewRouter(Options{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/skills/rag.query:invoke",
		strings.NewReader(`{"input": {"query": "hello", "top_k": 2}, "trace": {"trace_id": "t1"}}`),
	)
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
		Ok     bool `json:"ok"`
		Output struct {
			Hits []struct {
				DocID   string  `json:"doc_id"`
				ChunkID string  `json:"chunk_id"`
				Title   string  `json:"title"`
				URL     string  `json:"url"`
				Snippet string  `json:"snippet"`
				Score   float64 `json:"score"`
				Source  string  `json:"source"`
			} `json:"hits"`
		} `json:"output"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Ok != true {
		t.Fatalf("expected ok=true, got ok=%v err=%v", got.Ok, got.Error)
	}
	if len(got.Output.Hits) < 1 {
		t.Fatalf("expected at least 1 hit, got %d", len(got.Output.Hits))
	}
	if got.Output.Hits[0].DocID == "" {
		t.Fatalf("expected hit.doc_id to be non-empty")
	}
	if got.Output.Hits[0].Snippet == "" {
		t.Fatalf("expected hit.snippet to be non-empty")
	}

	// Assert the implementation used our stub embedding vector.
	wantVec := []float64{0.1, 0.2, 0.3}
	if len(gotQdrantRequest.Vector) != len(wantVec) {
		t.Fatalf("expected qdrant vector %v, got %v", wantVec, gotQdrantRequest.Vector)
	}
	for i := range wantVec {
		if gotQdrantRequest.Vector[i] != wantVec[i] {
			t.Fatalf("expected qdrant vector %v, got %v", wantVec, gotQdrantRequest.Vector)
		}
	}
	if gotQdrantRequest.Limit != 2 {
		t.Fatalf("expected qdrant limit=2, got %d", gotQdrantRequest.Limit)
	}
}

func TestInvokeSkill_RAGQuery_MissingOrEmptyQuery_ReturnsInvalidInput(t *testing.T) {
	t.Setenv("EMBEDDING_PROVIDER", "stub")
	t.Setenv("EMBEDDING_STUB_VECTOR", "0.1,0.2,0.3")

	r := NewRouter(Options{})

	cases := []struct {
		name string
		body string
	}{
		{name: "missing", body: `{"input": {}}`},
		{name: "empty", body: `{"input": {"query": ""}}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/skills/rag.query:invoke", strings.NewReader(tc.body))
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
				Ok    bool `json:"ok"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if got.Ok != false {
				t.Fatalf("expected ok=false, got %v", got.Ok)
			}
			if got.Error.Code != "INVALID_INPUT" {
				t.Fatalf("expected error.code=INVALID_INPUT, got %q", got.Error.Code)
			}
		})
	}
}
