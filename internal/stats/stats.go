package stats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AppUsage struct {
	Date          string `json:"date"`
	TotalVisits   int64  `json:"total_visits"`
	UniqueDevices int64  `json:"unique_devices"`
}

type StatsStore struct {
	mu        sync.RWMutex
	current   *AppUsage
	dataDir   string
	deviceIDs map[string]bool
}

func NewStatsStore(dataDir string) *StatsStore {
	if dataDir == "" {
		dataDir = "./data/stats"
	}
	_ = os.MkdirAll(dataDir, 0755)

	s := &StatsStore{
		dataDir:   dataDir,
		deviceIDs: make(map[string]bool),
		current:   loadTodayUsage(dataDir),
	}
	return s
}

func (s *StatsStore) RecordVisit(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if s.current.Date != today {
		s.saveLocked()
		s.current = &AppUsage{
			Date: today,
		}
		s.deviceIDs = make(map[string]bool)
	}

	s.current.TotalVisits++
	if deviceID != "" && !s.deviceIDs[deviceID] {
		s.deviceIDs[deviceID] = true
		s.current.UniqueDevices = int64(len(s.deviceIDs))
	}
}

func (s *StatsStore) Today() AppUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return AppUsage{
		Date:          s.current.Date,
		TotalVisits:   s.current.TotalVisits,
		UniqueDevices: s.current.UniqueDevices,
	}
}

func (s *StatsStore) History(days int) []AppUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]AppUsage, 0, days)
	result = append(result, s.Today())

	for i := 1; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		usage := loadUsageForDate(s.dataDir, date)
		if usage != nil {
			result = append(result, *usage)
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
	if s.current == nil || s.current.TotalVisits == 0 {
		return
	}
	path := filepath.Join(s.dataDir, s.current.Date+".json")
	b, _ := json.MarshalIndent(s.current, "", "  ")
	_ = os.WriteFile(path, b, 0644)
}

func loadTodayUsage(dataDir string) *AppUsage {
	today := time.Now().Format("2006-01-02")
	usage := loadUsageForDate(dataDir, today)
	if usage != nil {
		return usage
	}
	return &AppUsage{
		Date: today,
	}
}

func loadUsageForDate(dataDir, date string) *AppUsage {
	path := filepath.Join(dataDir, date+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var usage AppUsage
	if err := json.Unmarshal(b, &usage); err != nil {
		return nil
	}
	return &usage
}

func HandleVisit(store *StatsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			DeviceID string `json:"device_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		store.RecordVisit(req.DeviceID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
		})
	}
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

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": msg,
	})
}
