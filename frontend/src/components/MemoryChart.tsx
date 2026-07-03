/**
 * components/MemoryChart.tsx
 *
 * Canvas-based rolling chart for Go runtime memory metrics.
 *
 * Design contract:
 * ────────────────────────────────────────────────────────────────────
 *  • Pure canvas rendering via Chart.js — NO SVG, NO DOM per datapoint.
 *  • The chart instance and data buffers live in useRef — never state.
 *  • chart.update('none') skips the animation engine — 60 FPS capable.
 *  • Displays three heap series: Allocated, In-Use, Idle.
 *  • A secondary right-side Y-axis shows live Goroutine count.
 *  • GC events are annotated as vertical hairline markers.
 *  • All values are converted from bytes → MiB for human readability.
 * ────────────────────────────────────────────────────────────────────
 */
import { useRef, useEffect, useCallback } from 'react'
import { Chart, type ChartDataset } from 'chart.js'
import type { SystemMetrics } from '../types/telemetry'

// ─── Constants ──────────────────────────────────────────────────────────────

const MAX_POINTS  = 120  // 2 min of history at 1 sample/sec
const BYTES_TO_MB = 1 / (1024 * 1024)

// Colour palette — chrome / steel tones matching the design system
const COLOR_ALLOC    = 'rgba(250, 250, 250, 0.90)'  // white
const COLOR_INUSE    = 'rgba(161, 161, 170, 0.75)'  // zinc-400
const COLOR_IDLE     = 'rgba(63,  63,  70,  0.70)'  // zinc-700
const COLOR_GOROUTINE = 'rgba(52, 211, 153, 0.80)'  // emerald-400

// ─── Types ───────────────────────────────────────────────────────────────────

interface Props {
  metrics: SystemMetrics | null
}

interface Buffers {
  labels:     string[]
  allocMB:    number[]
  inuseMB:    number[]
  idleMB:     number[]
  goroutines: number[]
  gcCounts:   number[]   // used to detect GC events (delta)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function makeBuffers(): Buffers {
  return { labels: [], allocMB: [], inuseMB: [], idleMB: [], goroutines: [], gcCounts: [] }
}

function toMB(bytes: number): number {
  return parseFloat((bytes * BYTES_TO_MB).toFixed(2))
}

function timeLabel(): string {
  return new Date().toLocaleTimeString('en-GB', { hour12: false })
}

// ─── Component ───────────────────────────────────────────────────────────────

export function MemoryChart({ metrics }: Props) {
  const canvasRef  = useRef<HTMLCanvasElement>(null)
  const chartRef   = useRef<Chart | null>(null)
  const bufsRef    = useRef<Buffers>(makeBuffers())
  const prevGcRef  = useRef<number>(0)    // last-seen NumGC value

  // ── Chart initialisation ──────────────────────────────────────────────────

  const initChart = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const ctx = canvas.getContext('2d')!

    const heapDatasets: ChartDataset<'line', number[]>[] = [
      {
        label:           'Heap Alloc (MiB)',
        data:            [],
        borderColor:     COLOR_ALLOC,
        borderWidth:     1.5,
        backgroundColor: 'rgba(250,250,250,0.04)',
        fill:            true,
        tension:         0.3,
        pointRadius:     0,
        yAxisID:         'yMem',
      },
      {
        label:           'Heap In-Use (MiB)',
        data:            [],
        borderColor:     COLOR_INUSE,
        borderWidth:     1,
        backgroundColor: 'transparent',
        fill:            false,
        tension:         0.3,
        pointRadius:     0,
        yAxisID:         'yMem',
      },
      {
        label:           'Heap Idle (MiB)',
        data:            [],
        borderColor:     COLOR_IDLE,
        borderWidth:     1,
        backgroundColor: 'transparent',
        fill:            false,
        tension:         0.3,
        pointRadius:     0,
        yAxisID:         'yMem',
      },
    ]

    const goroutineDataset: ChartDataset<'line', number[]> = {
      label:           'Goroutines',
      data:            [],
      borderColor:     COLOR_GOROUTINE,
      borderWidth:     1,
      backgroundColor: 'transparent',
      fill:            false,
      tension:         0.2,
      pointRadius:     0,
      yAxisID:         'yGo',
    }

    chartRef.current = new Chart(ctx, {
      type: 'line',
      data: { labels: [], datasets: [...heapDatasets, goroutineDataset] },
      options: {
        animation:           false,
        responsive:          true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { display: false },
          tooltip: {
            backgroundColor: 'rgba(9,9,11,0.95)',
            borderColor:     'rgba(39,39,42,1)',
            borderWidth:     1,
            titleColor:      '#71717a',
            bodyColor:       '#fafafa',
            padding:         10,
            callbacks: {
              label: (item) => {
                const v = typeof item.raw === 'number' ? item.raw.toFixed(2) : '—'
                const unit = item.datasetIndex === 3 ? '' : ' MiB'
                return `  ${item.dataset.label}: ${v}${unit}`
              },
            },
          },
        },
        scales: {
          x: {
            type: 'category',
            ticks: {
              maxTicksLimit:  5,
              maxRotation:    0,
              color:          '#3f3f46',
              font:           { size: 9, family: 'monospace' },
              callback(_value, index) {
                return index % 20 === 0 ? this.getLabelForValue(index) : ''
              },
            },
            grid:   { display: false },
            border: { display: false },
          },
          yMem: {
            position: 'left',
            ticks: {
              color:         '#71717a',
              font:          { size: 9, family: 'monospace' },
              callback: (v) => `${v}M`,
              maxTicksLimit: 5,
            },
            grid:   { color: 'rgba(255,255,255,0.04)' },
            border: { display: false },
          },
          yGo: {
            position: 'right',
            ticks: {
              color:         '#34d399',
              font:          { size: 9, family: 'monospace' },
              maxTicksLimit: 5,
            },
            grid:   { display: false },
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

  // ── Data updates ────────────────────────────────────────────────────────

  useEffect(() => {
    if (!metrics || !chartRef.current) return

    const bufs  = bufsRef.current
    const chart = chartRef.current

    // Push new sample
    bufs.labels.push(timeLabel())
    bufs.allocMB.push(toMB(metrics.heap_alloc_bytes))
    bufs.inuseMB.push(toMB(metrics.heap_inuse_bytes))
    bufs.idleMB.push(toMB(metrics.heap_idle_bytes))
    bufs.goroutines.push(metrics.num_goroutines)
    bufs.gcCounts.push(metrics.num_gc)

    // Evict oldest points once buffer is full
    if (bufs.labels.length > MAX_POINTS) {
      bufs.labels.shift()
      bufs.allocMB.shift()
      bufs.inuseMB.shift()
      bufs.idleMB.shift()
      bufs.goroutines.shift()
      bufs.gcCounts.shift()
    }

    // Push data arrays into Chart.js datasets
    chart.data.labels              = [...bufs.labels]
    chart.data.datasets[0].data    = [...bufs.allocMB]
    chart.data.datasets[1].data    = [...bufs.inuseMB]
    chart.data.datasets[2].data    = [...bufs.idleMB]
    chart.data.datasets[3].data    = [...bufs.goroutines]

    // Detect GC event for annotation (delta in num_gc)
    const gcOccurred = metrics.num_gc > prevGcRef.current
    prevGcRef.current = metrics.num_gc

    // Highlight GC events as a momentary spike in the alloc dataset
    // by colouring the last point marker red
    const allocDs = chart.data.datasets[0] as unknown as Record<string, unknown>
    const markers = (bufs.allocMB.map((_, i) =>
      (gcOccurred && i === bufs.allocMB.length - 1) ? 4 : 0
    ))
    allocDs.pointRadius          = markers
    allocDs.pointBackgroundColor = markers.map(r => r > 0 ? '#ef4444' : 'transparent')

    chart.update('none')
  }, [metrics])

  // ── Render ──────────────────────────────────────────────────────────────

  // Derive summary stats for the header row
  const allocMB   = metrics ? toMB(metrics.heap_alloc_bytes) : 0
  const sysMB     = metrics ? toMB(metrics.heap_sys_bytes)   : 0
  const goroutines = metrics?.num_goroutines ?? 0
  const gcCount    = metrics?.num_gc ?? 0

  return (
    <div className="flex flex-col h-full">
      {/* ── Stat strip ───────────────────────────────────────────── */}
      <div className="flex gap-6 mb-3 flex-shrink-0">
        <Stat label="Heap Alloc" value={`${allocMB.toFixed(1)} MiB`} />
        <Stat label="Heap Sys"   value={`${sysMB.toFixed(1)} MiB`}   />
        <Stat label="Goroutines" value={goroutines.toString()} accent="emerald" />
        <Stat label="GC Cycles"  value={gcCount.toString()} />
      </div>

      {/* ── Canvas ───────────────────────────────────────────────── */}
      <div className="flex-1 relative min-h-0">
        {!metrics && (
          <div className="absolute inset-0 flex items-center justify-center text-[10px] text-zinc-600 uppercase tracking-widest font-mono">
            Waiting for system metrics…
          </div>
        )}
        <canvas
          ref={canvasRef}
          style={{ display: 'block', width: '100%', height: '100%' }}
        />
      </div>
    </div>
  )
}

// ─── Stat pill ────────────────────────────────────────────────────────────────

function Stat({
  label,
  value,
  accent = 'zinc',
}: {
  label: string
  value: string
  accent?: 'zinc' | 'emerald'
}) {
  const valueColor = accent === 'emerald' ? 'text-emerald-400' : 'text-zinc-200'
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[9px] uppercase tracking-widest text-zinc-600 font-mono">{label}</span>
      <span className={`text-[11px] font-mono font-semibold ${valueColor}`}>{value}</span>
    </div>
  )
}
