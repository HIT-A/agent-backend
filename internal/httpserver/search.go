package httpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"hoa-agent-backend/internal/skills"
)

func handleBraveSearch(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input struct {
			Query string `json:"query"`
			Count int    `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if input.Query == "" {
			writeJSONError(w, http.StatusBadRequest, "query is required")
			return
		}

		if input.Count <= 0 {
			input.Count = 5
		}

		ctx := context.Background()
		results, err := skills.BraveSearch(ctx, input.Query, input.Count)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSONOK(w, map[string]any{"ok": true, "results": results})
	}
}

func handleBraveAnswer(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input struct {
			Query           string `json:"query"`
			Model           string `json:"model"`
			Country         string `json:"country"`
			Language        string `json:"language"`
			EnableCitations bool   `json:"enable_citations"`
			EnableResearch  bool   `json:"enable_research"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if input.Query == "" {
			writeJSONError(w, http.StatusBadRequest, "query is required")
			return
		}

		ctx := context.Background()
		result, err := skills.BraveAnswer(
			ctx,
			input.Query,
			input.Model,
			input.Country,
			input.Language,
			input.EnableCitations,
			input.EnableResearch,
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":     true,
			"answer": result.Answer,
			"model":  result.Model,
			"usage":  result.Usage,
		})
	}
}
