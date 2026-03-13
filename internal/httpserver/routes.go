package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"hoa-agent-backend/internal/jobs"
	"hoa-agent-backend/internal/skills"
)

type JobStore interface {
	Create(ctx context.Context, skillName string, input json.RawMessage) (*jobs.Job, error)
	Get(ctx context.Context, id string) (*jobs.Job, error)
	UpdateStatus(ctx context.Context, id string, status jobs.Status, output json.RawMessage, errMsg string) (*jobs.Job, error)
}

type Options struct {
	Jobs  JobStore
	Queue chan<- string
}

func NewRouter(opts Options) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutes(mux, opts)
	return mux
}

func RegisterRoutes(mux *http.ServeMux, opts Options) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Only GET/HEAD are allowed for /health.
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		case http.MethodHead:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			return
		default:
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/v1/skills", func(w http.ResponseWriter, r *http.Request) {
		// Only GET is allowed for /v1/skills.
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		reg := skills.NewRegistry()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Skills []skills.Skill `json:"skills"`
		}{Skills: reg.List()})
	})

	// ServeMux will redirect "/v1/jobs" to "/v1/jobs/" for GET/HEAD.
	// Explicitly handle it to avoid implicit redirects.
	mux.HandleFunc("/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
			return
		default:
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/v1/jobs/", func(w http.ResponseWriter, r *http.Request) {
		// Expected: /v1/jobs/{job_id}
		id := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
		if id == "" || strings.Contains(id, "/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if opts.Jobs == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		job, err := opts.Jobs.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, jobs.ErrNotFound) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Ok  bool      `json:"ok"`
			Job *jobs.Job `json:"job"`
		}{Ok: true, Job: job})
	})

	mux.HandleFunc("/v1/skills/", func(w http.ResponseWriter, r *http.Request) {
		// Expected: /v1/skills/{name}:invoke
		path := strings.TrimPrefix(r.URL.Path, "/v1/skills/")

		// Only handle invoke suffix for now.
		const suffix = ":invoke"
		if !strings.HasSuffix(path, suffix) {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		name := strings.TrimSuffix(path, suffix)
		if name == "" || strings.Contains(name, "/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Input map[string]any `json:"input"`
			Trace map[string]any `json:"trace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		reg := skills.NewRegistry()
		skill, ok := reg.Get(name)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		output, err := skill.Invoke(r.Context(), req.Input, req.Trace)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Ok     bool           `json:"ok"`
			Output map[string]any `json:"output"`
		}{Ok: true, Output: output})
	})
}
