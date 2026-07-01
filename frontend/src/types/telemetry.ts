/**
 * types/telemetry.ts
 *
 * Strictly typed mirrors of the Go backend's models.TelemetryPayload
 * and wsserver.WSMessage structs. Any change to the Go model MUST be
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
 * WSMessage is the outer WebSocket frame envelope.
 * type is always "telemetry_snapshot".
 * payloads contains one entry per monitored endpoint.
 */
export interface WSMessage {
  type: 'telemetry_snapshot';
  timestamp: string;
  payloads: TelemetryPayload[];
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
