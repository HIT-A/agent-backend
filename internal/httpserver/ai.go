package httpserver

import (
	"encoding/json"
	"net/http"

	"hoa-agent-backend/internal/ai"
)

func handleAIChat(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input struct {
			Message string `json:"message"`
			System  string `json:"system,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		client := ai.NewClient()
		ctx := r.Context()

		system := input.System
		if system == "" {
			system = "You are a helpful AI assistant."
		}

		response, err := client.SimpleChat(ctx, system, input.Message)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":       true,
			"response": response,
		})
	}
}

func handleReActChat(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		client := ai.NewClient()
		ctx := r.Context()

		registry := opts.MCPRegistry
		if registry == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "MCP registry not configured")
			return
		}

		tools := ai.CreateDefaultTools(registry).GetAll()

		response, err := client.ReActChat(ctx, input.Message, tools)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":       true,
			"response": response,
		})
	}
}
