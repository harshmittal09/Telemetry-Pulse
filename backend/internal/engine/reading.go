package engine

import "math"

// Reading is what the dashboard receives, both over the websocket and from /api/history.
type Reading struct {
	URL        string  `json:"url"`
	LatencyMs  float64 `json:"latency_ms"`
	Status     int     `json:"status"`
	OK         bool    `json:"ok"`
	Error      string  `json:"error,omitempty"`
	ZScore     float64 `json:"zscore"`
	BaselineMs float64 `json:"baseline_ms"`
	Anomaly    bool    `json:"anomaly"`
	Timestamp  int64   `json:"timestamp"`
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
