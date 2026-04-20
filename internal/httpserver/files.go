package httpserver

import (
	"encoding/json"
	"net/http"

	"hoa-agent-backend/internal/skills"
)

func handleFilesUpload(opts Options) http.HandlerFunc {
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

		storage := opts.COSStorage
		if storage == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "COS storage not configured")
			return
		}

		skill := skills.NewFilesUploadSkill(storage)
		output, err := skill.Invoke(r.Context(), input, nil)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{"ok": true, "result": output})
	}
}

func handleFilesDownload(opts Options) http.HandlerFunc {
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

		storage := opts.COSStorage
		if storage == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "COS storage not configured")
			return
		}

		skill := skills.NewFilesDownloadSkill(storage)
		output, err := skill.Invoke(r.Context(), input, nil)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{"ok": true, "result": output})
	}
}

func handleFilesList(opts Options) http.HandlerFunc {
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

		storage := opts.COSStorage
		if storage == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "COS storage not configured")
			return
		}

		prefix, _ := input["prefix"].(string)
		maxKeys := 100
		if v, ok := input["max_keys"].(float64); ok {
			maxKeys = int(v)
		}

		files, err := storage.ListFiles(r.Context(), prefix, maxKeys)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":     true,
			"files":  files,
			"prefix": prefix,
			"count":  len(files),
		})
	}
}

func handleFilesDelete(opts Options) http.HandlerFunc {
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

		storage := opts.COSStorage
		if storage == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "COS storage not configured")
			return
		}

		key, _ := input["key"].(string)
		if key == "" {
			writeJSONOK(w, map[string]any{
				"ok":    false,
				"error": map[string]any{"code": "INVALID_INPUT", "message": "key is required"},
			})
			return
		}

		err := storage.DeleteFile(r.Context(), key)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":      true,
			"deleted": true,
			"key":     key,
		})
	}
}
