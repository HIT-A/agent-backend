package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"hoa-agent-backend/internal/skills"
)

// handleCoursesSearch handles POST /api/courses/search
func handleCoursesSearch(opts Options) http.HandlerFunc {
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

		skill := skills.NewCoursesSearchSkill()
		output, err := skill.Invoke(r.Context(), input, nil)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":      true,
			"results": output,
		})
	}
}

// handleCourseRead handles POST /api/courses/read
func handleCourseRead(opts Options) http.HandlerFunc {
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

		skill := skills.NewCourseReadSkill()
		output, err := skill.Invoke(r.Context(), input, nil)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":     true,
			"course": output,
		})
	}
}

// writeInvokeError translates a skill InvokeError into a JSON error response.
func writeInvokeError(w http.ResponseWriter, err error) {
	var invErr *skills.InvokeError
	if errors.As(err, &invErr) {
		writeJSONOK(w, map[string]any{
			"ok":    false,
			"error": map[string]any{"code": invErr.Code, "message": invErr.Message, "retryable": invErr.Retryable},
		})
		return
	}
	writeJSONOK(w, map[string]any{
		"ok":    false,
		"error": map[string]any{"code": "INTERNAL", "message": err.Error(), "retryable": true},
	})
}

// writeJSONOK writes a 200 JSON response.
func writeJSONOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONError writes a non-200 JSON error response.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": "HTTP_ERROR", "message": msg},
	})
}
