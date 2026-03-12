package httpserver

import "net/http"

func NewRouter() http.Handler {
	mux := http.NewServeMux()
	RegisterRoutes(mux)
	return mux
}

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}
