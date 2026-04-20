package httpserver

import (
	"encoding/json"
	"net/http"

	"hoa-agent-backend/internal/skills"
)

func handleCrawlPage(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		registry := opts.MCPRegistry
		if registry == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "MCP registry not configured")
			return
		}

		skill := skills.NewCrawl4AIPageSkill(registry)
		output, err := skill.Invoke(r.Context(), input, nil)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{"ok": true, "result": output})
	}
}

func handleCrawlSite(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		registry := opts.MCPRegistry
		if registry == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "MCP registry not configured")
			return
		}

		skill := skills.NewCrawl4AISiteSkill(registry)
		output, err := skill.Invoke(r.Context(), input, nil)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{"ok": true, "result": output})
	}
}

func handleCrawlStatus(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		registry := opts.MCPRegistry
		if registry == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "MCP registry not configured")
			return
		}

		skill := skills.NewCrawl4AIStatusSkill(registry)
		output, err := skill.Invoke(r.Context(), input, nil)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{"ok": true, "status": output})
	}
}
