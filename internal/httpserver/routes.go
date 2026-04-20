package httpserver

import (
	"net/http"

	"hoa-agent-backend/internal/mcp"
	"hoa-agent-backend/internal/stats"
	"hoa-agent-backend/internal/tempstore"
)

type Options struct {
	MCPRegistry *mcp.Registry
	TempStore   *tempstore.Store
	StatsStore  *stats.StatsStore
}

func NewRouter(opts Options) http.Handler {
	mux := http.NewServeMux()
	RegisterRoutes(mux, opts)
	return mux
}

func RegisterRoutes(mux *http.ServeMux, opts Options) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case http.MethodHead:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		default:
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/courses/search", handleCoursesSearch(opts))
	mux.HandleFunc("/api/courses/read", handleCourseRead(opts))
	mux.HandleFunc("/api/rag/query", handleRAGQuery(opts))
	mux.HandleFunc("/api/rag/ingest", handleRAGIngest(opts))
	mux.HandleFunc("/api/crawl/page", handleCrawlPage(opts))
	mux.HandleFunc("/api/crawl/site", handleCrawlSite(opts))
	mux.HandleFunc("/api/search/brave", handleBraveSearch(opts))
	mux.HandleFunc("/api/search/brave/answer", handleBraveAnswer(opts))
	mux.HandleFunc("/api/crawl/status", handleCrawlStatus(opts))
	mux.HandleFunc("/api/teachers/search", handleTeacherSearch(opts))
	mux.HandleFunc("/api/hitsz/fetch", handleHITSZFetch(opts))

	mux.HandleFunc("/api/ai/chat", handleAIChat(opts))
	mux.HandleFunc("/api/ai/react", handleReActChat(opts))

	mux.HandleFunc("/api/temp/upload", handleTempUpload(opts))
	mux.HandleFunc("/api/temp/download", handleTempDownload(opts))
	mux.HandleFunc("/api/temp/parse", handleTempParse(opts))
	mux.HandleFunc("/api/temp/list", handleTempList(opts))
	mux.HandleFunc("/api/stats", stats.HandleStats(opts.StatsStore))
	mux.HandleFunc("/api/visit", stats.HandleVisit(opts.StatsStore))
}
