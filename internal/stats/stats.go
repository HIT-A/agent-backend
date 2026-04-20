package stats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type DailyStats struct {
	Date         string           `json:"date"`
	TotalCalls   int64            `json:"total_calls"`
	EndpointHits map[string]int64 `json:"endpoint_hits"`
	UniqueIPs    map[string]bool  `json:"-"`
	IPCount      int64            `json:"unique_ips"`
}

type StatsStore struct {
	mu      sync.RWMutex
	current *DailyStats
	dataDir string
}

func NewStatsStore(dataDir string) *StatsStore {
	if dataDir == "" {
		dataDir = "./data/stats"
	}
	_ = os.MkdirAll(dataDir, 0755)

	s := &StatsStore{
		dataDir: dataDir,
		current: loadTodayStats(dataDir),
	}
	return s
}

func (s *StatsStore) Record(endpoint string, clientIP string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if s.current.Date != today {
		s.saveLocked()
		s.current = &DailyStats{
			Date:         today,
			EndpointHits: make(map[string]int64),
			UniqueIPs:    make(map[string]bool),
		}
	}

	s.current.TotalCalls++
	s.current.EndpointHits[endpoint]++

	if clientIP != "" && !s.current.UniqueIPs[clientIP] {
		s.current.UniqueIPs[clientIP] = true
		s.current.IPCount = int64(len(s.current.UniqueIPs))
	}
}

func (s *StatsStore) Today() DailyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return DailyStats{
		Date:         s.current.Date,
		TotalCalls:   s.current.TotalCalls,
		EndpointHits: copyMap(s.current.EndpointHits),
		IPCount:      s.current.IPCount,
	}
}

func (s *StatsStore) History(days int) []DailyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]DailyStats, 0, days)
	result = append(result, s.Today())

	for i := 1; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		stat := loadStatsForDate(s.dataDir, date)
		if stat != nil {
			result = append(result, *stat)
		}
	}
	return result
}

func (s *StatsStore) Save() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveLocked()
}

func (s *StatsStore) saveLocked() {
	if s.current == nil || s.current.TotalCalls == 0 {
		return
	}
	path := filepath.Join(s.dataDir, s.current.Date+".json")
	b, _ := json.MarshalIndent(s.current, "", "  ")
	_ = os.WriteFile(path, b, 0644)
}

func loadTodayStats(dataDir string) *DailyStats {
	today := time.Now().Format("2006-01-02")
	stat := loadStatsForDate(dataDir, today)
	if stat != nil {
		stat.UniqueIPs = make(map[string]bool)
		return stat
	}
	return &DailyStats{
		Date:         today,
		EndpointHits: make(map[string]int64),
		UniqueIPs:    make(map[string]bool),
	}
}

func loadStatsForDate(dataDir, date string) *DailyStats {
	path := filepath.Join(dataDir, date+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var stat DailyStats
	if err := json.Unmarshal(b, &stat); err != nil {
		return nil
	}
	return &stat
}

func copyMap(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TrackingMiddleware(next http.Handler, store *StatsStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store != nil && strings.HasPrefix(r.URL.Path, "/api/") {
			ip := extractIP(r)
			store.Record(r.URL.Path, ip)
		}
		next.ServeHTTP(w, r)
	})
}

func extractIP(r *http.Request) string {
	fwd := r.Header.Get("X-Forwarded-For")
	if fwd != "" {
		parts := strings.Split(fwd, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

func HandleStats(store *StatsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		days := 7
		if d := r.URL.Query().Get("days"); d != "" {
			if n, err := fmt.Sscanf(d, "%d", &days); err == nil && n == 1 {
				if days < 1 {
					days = 1
				}
				if days > 30 {
					days = 30
				}
			}
		}

		history := store.History(days)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"data": history,
		})
	}
}
