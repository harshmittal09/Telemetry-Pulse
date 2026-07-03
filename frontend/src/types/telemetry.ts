/**
 * types/telemetry.ts
 *
 * Strictly typed mirrors of the Go backend's models package and
 * wsserver.WSMessage struct. Any change to the Go models MUST be
 * reflected here to maintain the contract.
 */

/**
 * TelemetryPayload is the per-endpoint datum received from the backend.
 * Field names match the Go JSON tags exactly (snake_case).
 */
export interface TelemetryPayload {
  endpoint_id: string;
  target: string;
  protocol: 'icmp' | 'tcp';
  timestamp: string;       // ISO-8601 UTC
  latency_ms: number;
  jitter_ms: number;
  packet_loss: number;     // 0–100 %
  mean_ms: number;         // μ
  std_dev_ms: number;      // σ
  z_score: number;         // Z = (x − μ) / σ
  is_anomaly: boolean;     // |Z| >= anomaly_threshold
  anomaly_threshold: number;
}

/**
 * SystemMetrics mirrors models.SystemMetrics from the Go backend.
 * Captures a point-in-time snapshot of the Go process's runtime memory
 * and GC state, collected via runtime.MemStats.
 */
export interface SystemMetrics {
  timestamp: string;           // ISO-8601 UTC

  // Heap allocation (bytes)
  heap_alloc_bytes: number;
  heap_sys_bytes: number;
  heap_inuse_bytes: number;
  heap_idle_bytes: number;
  heap_released_bytes: number;

  // GC statistics
  num_gc: number;
  num_forced_gc: number;
  pause_total_ns: number;
  last_gc_pause_ns: number;

  // Allocation throughput
  total_alloc_bytes: number;
  malloc_count: number;
  free_count: number;

  // Concurrency
  num_goroutines: number;
}

/**
 * RDSMetrics mirrors models.RDSMetrics from the Go backend.
 * Represents a point-in-time snapshot of an AWS RDS cluster's backup
 * health and availability state.
 */
export interface RDSMetrics {
  timestamp: string;           // ISO-8601 UTC
  cluster_id: string;

  // Cluster identity
  engine: string;
  engine_version: string;
  multi_az: boolean;

  // Availability
  status: string;              // "available" | "backing-up" | "failing-over" | ...

  // Continuous backup (PITR)
  backup_retention_days: number;
  preferred_backup_window: string;
  earliest_restorable_time: string;  // ISO-8601 UTC
  latest_restorable_time: string;    // ISO-8601 UTC
  /** Seconds behind the latest restorable point. >300s = alert condition. */
  backup_lag_seconds: number;

  // Automated snapshot inventory
  automated_snapshot_count: number;
  completed_snapshot_count: number;
  latest_snapshot_time: string;        // ISO-8601 UTC
  latest_snapshot_status: string;
  latest_snapshot_encrypted: boolean;
  latest_snapshot_size_gb: number;

  // Populated when the probe fails — non-empty string = degraded state
  probe_error?: string;
  snapshot_query_error?: string;
}

/**
 * WSMessage is the outer WebSocket frame envelope.
 * Mirrors wsserver.WSMessage in hub.go exactly.
 */
export interface WSMessage {
  type: 'telemetry_snapshot';
  timestamp: string;
  payloads: TelemetryPayload[];
  /** Present on every frame (Go runtime metrics). */
  system?: SystemMetrics;
  /** Present only when the RDS probe is configured (RDS_CLUSTER_ID env var). */
  rds?: RDSMetrics;
}

/**
 * EndpointState is the latest TelemetryPayload per endpoint,
 * indexed by endpoint_id. Stored in React state and consumed by
 * all dashboard components.
 */
export type EndpointStateMap = Map<string, TelemetryPayload>;

/**
 * LogEntry is a timestamped record added to the virtualized event log
 * on every broadcast. seq is a monotonically increasing sequence number
 * used as the stable React key.
 */
export interface LogEntry {
  seq: number;
  payload: TelemetryPayload;
}

/** WebSocket connection lifecycle states. */
export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected';
