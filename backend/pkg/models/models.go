package models

import "time"

// EndpointID is a strongly typed identifier for a monitored network endpoint.
type EndpointID string

// ProbeResult represents the raw output from a single network probe cycle.
type ProbeResult struct {
	EndpointID  EndpointID `json:"endpoint_id"`
	Target      string     `json:"target"`
	Protocol    string     `json:"protocol"` // "icmp" | "tcp"
	Latency     float64    `json:"latency_ms"`
	PacketLoss  float64    `json:"packet_loss_percent"` // 0–100
	IsReachable bool       `json:"is_reachable"`
	Timestamp   time.Time  `json:"timestamp"`
}

// WindowStats holds the O(1) running statistics for a sliding window.
// All fields are safe for concurrent read after the window's mutex is held.
type WindowStats struct {
	// N is the configured window size (e.g. 50).
	N int

	// Count is the number of samples currently in the window (≤ N).
	Count int

	// RunningSum is Σ(xᵢ) across all samples in the current window.
	RunningSum float64

	// RunningSumSq is Σ(xᵢ²) across all samples in the current window.
	RunningSumSq float64

	// Mean is the current Moving Mean μ = RunningSum / Count.
	Mean float64

	// Variance is the current Moving Variance σ² = E[x²] - (E[x])².
	Variance float64

	// StdDev is √σ².
	StdDev float64
}

// TelemetryPayload is the fully enriched datum published to Redis pub/sub
// and streamed over WebSocket to the frontend. It is the single source of
// truth for a given probe cycle.
type TelemetryPayload struct {
	EndpointID   EndpointID `json:"endpoint_id"`
	Target       string     `json:"target"`
	Protocol     string     `json:"protocol"`
	Timestamp    time.Time  `json:"timestamp"`
	LatencyMs    float64    `json:"latency_ms"`
	Jitter       float64    `json:"jitter_ms"`    // |current - previous| latency
	PacketLoss   float64    `json:"packet_loss"`  // 0–100 %
	Mean         float64    `json:"mean_ms"`      // μ
	StdDev       float64    `json:"std_dev_ms"`   // σ
	ZScore       float64    `json:"z_score"`      // Z = (x - μ) / σ
	IsAnomaly    bool       `json:"is_anomaly"`   // |Z| >= AnomalyThreshold
	AnomalyThreshold float64 `json:"anomaly_threshold"`
}

// EndpointConfig describes a single monitored target.
type EndpointConfig struct {
	ID           EndpointID `json:"id"`
	Target       string     `json:"target"`
	Protocol     string     `json:"protocol"`
	ProbeIntervalMs int     `json:"probe_interval_ms"`
}
