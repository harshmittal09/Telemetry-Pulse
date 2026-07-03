// Package models contains the shared data structures for telemetry and AWS probes.

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
	ID              EndpointID `json:"id"`
	Target          string     `json:"target"`
	Protocol        string     `json:"protocol"`
	ProbeIntervalMs int        `json:"probe_interval_ms"`
}

// ---------------------------------------------------------------------------
// System Metrics (Go runtime)
// ---------------------------------------------------------------------------

// SystemMetrics holds a point-in-time snapshot of the Go process's memory
// and concurrency state, captured via runtime.MemStats.
type SystemMetrics struct {
	Timestamp time.Time `json:"timestamp"`

	// Heap allocation (bytes)
	HeapAllocBytes    uint64 `json:"heap_alloc_bytes"`
	HeapSysBytes      uint64 `json:"heap_sys_bytes"`
	HeapInuseBytes    uint64 `json:"heap_inuse_bytes"`
	HeapIdleBytes     uint64 `json:"heap_idle_bytes"`
	HeapReleasedBytes uint64 `json:"heap_released_bytes"`

	// GC statistics
	NumGC         uint32 `json:"num_gc"`
	NumForcedGC   uint32 `json:"num_forced_gc"`
	PauseTotalNs  uint64 `json:"pause_total_ns"`
	LastGCPauseNs uint64 `json:"last_gc_pause_ns"`

	// Allocation throughput
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	MallocCount     uint64 `json:"malloc_count"`
	FreeCount       uint64 `json:"free_count"`

	// Concurrency
	NumGoroutines uint64 `json:"num_goroutines"`
}

// ---------------------------------------------------------------------------
// RDS Metrics (AWS RDS / Aurora)
// ---------------------------------------------------------------------------

// RDSMetrics holds a point-in-time snapshot of an AWS RDS cluster's
// backup health, availability status, and snapshot inventory.
type RDSMetrics struct {
	Timestamp time.Time `json:"timestamp"`
	ClusterID string    `json:"cluster_id"`

	// Cluster identity
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	MultiAZ       bool   `json:"multi_az"`

	// Availability
	Status string `json:"status"` // "available", "backing-up", "failing-over", etc.

	// Continuous backup (Point-in-Time Recovery)
	BackupRetentionDays   int       `json:"backup_retention_days"`
	PreferredBackupWindow string    `json:"preferred_backup_window"`
	EarliestRestorableTime time.Time `json:"earliest_restorable_time"`
	LatestRestorableTime   time.Time `json:"latest_restorable_time"`
	// BackupLagSeconds is how far behind the latest restorable point is from now.
	// A value > 300s on a healthy Aurora cluster should trigger an alert.
	BackupLagSeconds float64 `json:"backup_lag_seconds"`

	// Automated snapshot inventory
	AutomatedSnapshotCount  int       `json:"automated_snapshot_count"`
	CompletedSnapshotCount  int       `json:"completed_snapshot_count"`
	LatestSnapshotTime      time.Time `json:"latest_snapshot_time"`
	LatestSnapshotStatus    string    `json:"latest_snapshot_status"`
	LatestSnapshotEncrypted bool      `json:"latest_snapshot_encrypted"`
	LatestSnapshotSizeGB    int32     `json:"latest_snapshot_size_gb"`

	// Error fields — populated on probe failure, non-zero means degraded.
	ProbeError         string `json:"probe_error,omitempty"`
	SnapshotQueryError string `json:"snapshot_query_error,omitempty"`
}
