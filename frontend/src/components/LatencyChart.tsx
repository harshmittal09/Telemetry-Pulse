/**
 * components/LatencyChart.tsx
 *
 * Canvas-based streaming latency chart using Chart.js v4.
 *
 * Design contract:
 * ─────────────────────────────────────────────────────────────────────
 *  • Uses a <canvas> element — NO SVG, NO DOM manipulation per datapoint.
 *  • All data management is IMPERATIVE via Chart.js refs.
 *  • chart.update('none') is called to skip animations entirely —
 *    critical for smooth 60 FPS rendering at 2 Hz data ingest.
 *  • The chart instance lives in chartRef (useRef) — never in React state.
 *  • Rolling data buffers (MAX_CHART_POINTS per endpoint) live in buffersRef.
 *  • Per-point anomaly colouring: anomalous points render as solid red dots.
 *  • State data flows in via `endpoints` prop (from useTelemetryStream).
 *    When `endpoints` reference changes, a useEffect pushes new data into
 *    the buffers and calls chart.update('none').
 *
 * Performance invariants:
 *  • O(1) push + shift per data point (Array.push + Array.shift).
 *  • chart.update('none') skips Chart.js's Tween animation engine entirely.
 *  • The canvas uses GPU-accelerated 2D compositing — zero DOM mutation.
 * ─────────────────────────────────────────────────────────────────────
 */
import { useRef, useEffect, useCallback } from 'react'
import {
  Chart,
  type ChartDataset,
} from 'chart.js'
import type { EndpointStateMap } from '../types/telemetry'
import {
  ENDPOINT_IDS,
  ENDPOINT_META,
  MAX_CHART_POINTS,
  ANOMALY_COLOR,
} from '../lib/constants'

// ─── Types ─────────────────────────────────────────────────────────────────

/** Rolling data buffer for one endpoint. */
interface DataBuffer {
  latency:     number[]
  pointColors: string[]    // '#ef4444' for anomaly, 'transparent' for normal
  pointRadii:  number[]    // 5 for anomaly, 0 for normal
  labels:      string[]    // HH:MM:SS for each sample
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function makeEmptyBuffer(): DataBuffer {
  return { latency: [], pointColors: [], pointRadii: [], labels: [] }
}

function nowLabel(): string {
  return new Date().toLocaleTimeString('en-GB', { hour12: false })
}

// ─── Component ──────────────────────────────────────────────────────────────

interface LatencyChartProps {
  endpoints: EndpointStateMap
}

export function LatencyChart({ endpoints }: LatencyChartProps) {
  const canvasRef  = useRef<HTMLCanvasElement>(null)
  const chartRef   = useRef<Chart | null>(null)

  /** Rolling data buffers — one per endpoint, keyed by endpoint_id. */
  const buffersRef = useRef<Map<string, DataBuffer>>(
    new Map(ENDPOINT_IDS.map(id => [id, makeEmptyBuffer()]))
  )

  /** Track the last-seen timestamp per endpoint to avoid duplicate pushes. */
  const lastTsRef  = useRef<Map<string, string>>(new Map())

  // ── Chart initialisation (runs once on mount) ──────────────────────────

  const initChart = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const ctx = canvas.getContext('2d')!

    // Build initial (empty) datasets — one per endpoint.
    const datasets: ChartDataset<'line', number[]>[] = ENDPOINT_IDS.map(id => {
      const meta = ENDPOINT_META[id]
      return {
        label:               meta.label,
        data:                [],
        borderColor:         meta.color,
        borderWidth:         1.5,
        // Point styling uses per-point arrays to highlight anomalies.
        pointRadius:         [] as unknown as number,
        pointBackgroundColor: [] as unknown as string,
        pointBorderColor:    'transparent',
        pointHoverRadius:    4,
        tension:             0.35,
        fill:                false,
        backgroundColor:     'transparent',
        // Clip data to the chart area so points don't overflow.
        clip:                false,
      }
    })

    chartRef.current = new Chart(ctx, {
      type: 'line',
      data: { labels: [], datasets },
      options: {
        // ── Performance ─────────────────────────────────
        animation:          false,   // CRITICAL: skip tween engine at 2 Hz
        responsive:         true,
        maintainAspectRatio: false,

        // ── Interaction ──────────────────────────────────
        interaction: {
          mode:      'index',
          intersect: false,
        },

        // ── Plugins ──────────────────────────────────────
        plugins: {
          // Hide default legend — we render our own colour key in the header.
          legend: { display: false },

          tooltip: {
            backgroundColor:  'rgba(11, 17, 32, 0.95)',
            borderColor:      'rgba(30, 45, 69, 0.8)',
            borderWidth:      1,
            titleColor:       '#7a90b0',
            bodyColor:        '#f0f6ff',
            padding:          10,
            caretSize:        6,
            callbacks: {
              title: (items) => `t = ${items[0]?.label ?? ''}`,
              label: (item) => {
                const v = typeof item.raw === 'number' ? item.raw.toFixed(2) : '—'
                return `  ${item.dataset.label}: ${v} ms`
              },
            },
          },
        },

        // ── Scales ────────────────────────────────────────
        scales: {
          x: {
            display:        true,
            type:           'category',
            ticks: {
              maxTicksLimit:  6,
              maxRotation:    0,
              color:          '#3d5166',
              font:           { size: 10, family: 'monospace' },
              // Show every 10th label to avoid crowding.
              callback(_value, index) {
                return index % 10 === 0
                  ? this.getLabelForValue(index)
                  : ''
              },
            },
            grid: {
              display:       false,
              drawOnChartArea: false,
              drawTicks:     false,
            },
            border: { display: false },
          },

          y: {
            position: 'left',
            ticks: {
              color:    '#71717a',
              font:     { size: 10, family: 'monospace' },
              callback: (v) => `${v}ms`,
              maxTicksLimit: 6,
            },
            grid: {
              color:  'rgba(255, 255, 255, 0.05)',
            },
            border: { display: false },
          },
        },
      },
    })
  }, [])

  useEffect(() => {
    initChart()
    return () => {
      chartRef.current?.destroy()
      chartRef.current = null
    }
  }, [initChart])

  // ── Data update (runs when the endpoints snapshot changes) ─────────────
  // `endpoints` is a NEW Map reference on each rAF flush that contains
  // new data — so this effect fires at most at 60 FPS, but in practice
  // only when the 500ms WS broadcast delivers a new snapshot.

  useEffect(() => {
    const chart = chartRef.current
    if (!chart || endpoints.size === 0) return

    let hasNewData = false

    for (const id of ENDPOINT_IDS) {
      const payload = endpoints.get(id)
      if (!payload) continue

      // Skip if this exact timestamp was already processed.
      if (lastTsRef.current.get(id) === payload.timestamp) continue
      lastTsRef.current.set(id, payload.timestamp)
      hasNewData = true

      const buf = buffersRef.current.get(id)!

      // Push incoming sample into the rolling buffer.
      buf.latency.push(payload.latency_ms)
      buf.pointColors.push(payload.is_anomaly ? ANOMALY_COLOR : 'transparent')
      buf.pointRadii.push(payload.is_anomaly ? 5 : 0)
      buf.labels.push(nowLabel())

      // Evict oldest point when buffer exceeds MAX_CHART_POINTS.
      if (buf.latency.length > MAX_CHART_POINTS) {
        buf.latency.shift()
        buf.pointColors.shift()
        buf.pointRadii.shift()
        buf.labels.shift()
      }
    }

    if (!hasNewData) return

    // Synchronise chart datasets from the buffers.
    // Use ep-01's labels as the shared x-axis (all endpoints share the same cadence).
    const sharedLabels = buffersRef.current.get('ep-01')?.labels ?? []
    chart.data.labels = [...sharedLabels]

    ENDPOINT_IDS.forEach((id, datasetIndex) => {
      const buf     = buffersRef.current.get(id)!
      const dataset = chart.data.datasets[datasetIndex] as ChartDataset<'line', number[]>

      dataset.data = [...buf.latency]
      // Chart.js v4 accepts per-point arrays for these properties.
      // Use unknown intermediary to satisfy TypeScript's strict overlap check.
      ;(dataset as unknown as Record<string, unknown>).pointBackgroundColor = [...buf.pointColors]
      ;(dataset as unknown as Record<string, unknown>).pointRadius          = [...buf.pointRadii]
    })

    // CRITICAL: 'none' mode skips the Tween animation engine entirely.
    // This is what allows smooth Canvas rendering at streaming frequency.
    chart.update('none')

  }, [endpoints])

  // ── Render ──────────────────────────────────────────────────────────────

  return (
    <div style={{
      position:   'relative',
      width:      '100%',
      height:     '100%',
      minHeight:  0,
    }}>
      <canvas ref={canvasRef} style={{ display: 'block', width: '100%', height: '100%' }} />
    </div>
  )
}
