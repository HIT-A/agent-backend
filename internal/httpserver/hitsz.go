package httpserver

import (
	"encoding/json"
	"net/http"
	"os"

	"hoa-agent-backend/internal/hitsz"
)

func getDefaultHITSZCredentials() (username, password string) {
	return os.Getenv("HITSZ_USERNAME"), os.Getenv("HITSZ_PASSWORD")
}

func handleHITSZFetch(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if input.URL == "" {
			input.URL = "https://info.hitsz.edu.cn/list.jsp?wbtreeid=1053"
		}

		username, password := getDefaultHITSZCredentials()
		if username == "" || password == "" {
			writeJSONOK(w, map[string]any{
				"ok":    false,
				"error": map[string]any{"code": "CONFIG", "message": "HITSZ credentials not configured"},
			})
			return
		}

		result, err := hitsz.FetchPublicInfo(username, password, input.URL)
		if err != nil {
			writeInvokeError(w, err)
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":            true,
			"login_success": result.LoginSuccess,
			"page":          result.FetchedPage,
		})
	}
}
