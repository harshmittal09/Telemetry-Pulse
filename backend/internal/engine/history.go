package engine

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// History keeps the last few readings per target in memory so a freshly
// opened dashboard can draw its charts straight away.
type History struct {
	mu       sync.RWMutex
	limit    int
	interval time.Duration
	targets  []string
	readings map[string][]Reading
}

func NewHistory(targets []string, limit int, interval time.Duration) *History {
	return &History{
		limit:    limit,
		interval: interval,
		targets:  targets,
		readings: make(map[string][]Reading),
	}
}

func (h *History) Add(r Reading) {
	h.mu.Lock()
	defer h.mu.Unlock()

	list := append(h.readings[r.URL], r)
	if len(list) > h.limit {
		copy(list, list[len(list)-h.limit:])
		list = list[:h.limit]
	}
	h.readings[r.URL] = list
}

type targetHistory struct {
	URL      string    `json:"url"`
	Readings []Reading `json:"readings"`
}

type historyResponse struct {
	IntervalMs int64           `json:"interval_ms"`
	Targets    []targetHistory `json:"targets"`
}

func (h *History) snapshot() historyResponse {
	h.mu.RLock()
	defer h.mu.RUnlock()

	resp := historyResponse{IntervalMs: h.interval.Milliseconds()}
	for _, url := range h.targets {
		readings := append([]Reading{}, h.readings[url]...)
		resp.Targets = append(resp.Targets, targetHistory{URL: url, Readings: readings})
	}
	return resp
}

// ServeHTTP handles GET /api/history
func (h *History) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(h.snapshot())
}
