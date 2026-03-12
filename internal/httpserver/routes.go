package httpserver

import (
	"encoding/json"
	"net/http"

	"hoa-agent-backend/internal/skills"
)

func NewRouter() http.Handler {
	mux := http.NewServeMux()
	RegisterRoutes(mux)
	return mux
}

func RegisterRoutes(mux *http.ServeMux) {
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
}
