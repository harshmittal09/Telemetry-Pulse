/**
 * lib/constants.ts
 * Single source of truth for all chart/UI configuration constants.
 */

/** Number of rolling data points kept per endpoint in the latency chart. */
export const MAX_CHART_POINTS = 60

/** Maximum log entries retained in the virtual log. */
export const MAX_LOG_ENTRIES = 2000

/** Fixed pixel height of each row in the VirtualLog. */
export const LOG_ROW_HEIGHT = 38

/** Z-score anomaly threshold — must match backend DefaultThreshold. */
export const ANOMALY_THRESHOLD = 2.5

/** Per-endpoint visual identity — order must be stable. */
export const ENDPOINT_META: Record<string, { color: string; colorAlpha: string; label: string }> = {
  'ep-01': {
    color:      '#d4d4d8', // zinc-300
    colorAlpha: 'transparent',
    label:      '8.8.8.8 · ICMP',
  },
  'ep-02': {
    color:      '#71717a', // zinc-500
    colorAlpha: 'transparent',
    label:      '1.1.1.1 · ICMP',
  },
  'ep-03': {
    color:      '#3f3f46', // zinc-700
    colorAlpha: 'transparent',
    label:      'api.example.com · TCP',
  },
}

/** Ordered list of endpoint IDs (for deterministic dataset indexing). */
export const ENDPOINT_IDS = Object.keys(ENDPOINT_META)

/** Anomaly point highlight colour. */
export const ANOMALY_COLOR = '#ef4444'
