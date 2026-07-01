/**
 * components/VirtualLog.tsx
 *
 * Virtualized event log using TanStack Virtual v3.
 * Phase 3 Refactor: Stark minimalistic event log. No background highlights,
 * purely typographical hierarchy and specific value red highlights for anomalies.
 */
import { useRef, memo } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { LogEntry } from '../types/telemetry'
import { LOG_ROW_HEIGHT, ENDPOINT_META, ANOMALY_THRESHOLD } from '../lib/constants'

const COL = {
  seq:      72,
  time:     74,
  endpoint: 68,
  latency:  80,
  zscore:   78,
  jitter:   74,
  status:   '1fr',
}

const GRID_COLS = `${COL.seq}px ${COL.time}px ${COL.endpoint}px ${COL.latency}px ${COL.zscore}px ${COL.jitter}px ${COL.status}`

function LogHeader() {
  return (
    <div
      style={{ gridTemplateColumns: GRID_COLS }}
      className="grid gap-x-2 px-3 py-2 text-[10px] font-bold tracking-widest uppercase text-zinc-500 border-b border-zinc-900 bg-black shrink-0 font-sans"
    >
      <span>#</span>
      <span>Time</span>
      <span>Endpoint</span>
      <span>Latency</span>
      <span>Z-Score</span>
      <span>Jitter</span>
      <span>Status</span>
    </div>
  )
}

interface LogRowProps {
  entry: LogEntry
}

const LogRow = memo(function LogRow({ entry }: LogRowProps) {
  const { seq, payload } = entry
  const { endpoint_id, latency_ms, z_score, jitter_ms, is_anomaly, timestamp } = payload

  const meta      = ENDPOINT_META[endpoint_id]
  const absZ      = Math.abs(z_score)
  const isWarn    = absZ >= ANOMALY_THRESHOLD * 0.75 && !is_anomaly
  const timeStr   = new Date(timestamp).toLocaleTimeString('en-GB', { hour12: false })

  return (
    <div
      style={{
        gridTemplateColumns: GRID_COLS,
        height: LOG_ROW_HEIGHT,
      }}
      className="grid items-center gap-x-2 px-3 border-b border-zinc-900 bg-transparent font-mono text-[11.5px] hover:bg-zinc-950 transition-colors"
    >
      <span className="text-[10px] text-zinc-600">
        {seq.toString().padStart(6, '0')}
      </span>

      <span className="text-zinc-500">
        {timeStr}
      </span>

      <span className="flex items-center gap-1.5">
        <span 
          style={{ background: meta?.color ?? '#fff' }} 
          className="w-1.5 h-1.5 rounded-full shrink-0" 
        />
        <span className="font-semibold text-white">
          {endpoint_id}
        </span>
      </span>

      <span className={`${is_anomaly ? 'text-red-500 font-bold' : 'text-zinc-300'}`}>
        {latency_ms.toFixed(2)}
        <span className="text-[9px] text-zinc-600"> ms</span>
      </span>

      <span className={`${absZ >= ANOMALY_THRESHOLD ? 'text-red-500 font-bold' : 'text-zinc-300'}`}>
        {z_score >= 0 ? '+' : ''}{z_score.toFixed(3)}
      </span>

      <span className="text-zinc-500">
        {jitter_ms.toFixed(2)}
        <span className="text-[9px] text-zinc-700"> ms</span>
      </span>

      <span className="flex items-center gap-1.5">
        {is_anomaly ? (
          <>
            <div className="w-1 h-1 rounded-full bg-red-500" />
            <span className="text-[10px] uppercase tracking-widest text-red-500 font-bold font-sans">Anomaly</span>
          </>
        ) : isWarn ? (
          <>
            <div className="w-1 h-1 rounded-full bg-zinc-400" />
            <span className="text-[10px] uppercase tracking-widest text-zinc-400 font-sans">Elevated</span>
          </>
        ) : (
          <>
            <div className="w-1 h-1 rounded-full bg-zinc-600" />
            <span className="text-[10px] uppercase tracking-widest text-zinc-600 font-sans">Normal</span>
          </>
        )}
      </span>
    </div>
  )
})

export function VirtualLog({ log }: { log: LogEntry[] }) {
  const parentRef = useRef<HTMLDivElement>(null)

  const rowVirtualizer = useVirtualizer({
    count:            log.length,
    getScrollElement: () => parentRef.current,
    estimateSize:     () => LOG_ROW_HEIGHT,
    overscan:         12,
  })

  const totalSize    = rowVirtualizer.getTotalSize()
  const virtualItems = rowVirtualizer.getVirtualItems()

  return (
    <div className="flex flex-col h-full overflow-hidden bg-black">
      <LogHeader />

      <div ref={parentRef} className="flex-1 overflow-y-auto relative">
        {log.length === 0 ? (
          <div className="py-8 text-center text-zinc-600 text-xs font-sans tracking-wide">
            Waiting for telemetry data…
          </div>
        ) : (
          <div style={{ height: totalSize }} className="w-full relative">
            {virtualItems.map(virtualRow => (
              <div
                key={virtualRow.key}
                style={{
                  position:  'absolute',
                  top:       0,
                  left:      0,
                  width:     '100%',
                  height:    `${virtualRow.size}px`,
                  transform: `translateY(${virtualRow.start}px)`,
                }}
              >
                <LogRow entry={log[virtualRow.index]} />
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="px-3 py-2 border-t border-zinc-900 flex justify-between items-center text-[10px] text-zinc-600 font-mono shrink-0 bg-black">
        <span>
          <span className="text-zinc-400">{log.length.toLocaleString()}</span>
          &nbsp;events · showing {virtualItems.length} rows in DOM
        </span>
        <span>
          {log.filter(e => e.payload.is_anomaly).length.toLocaleString()} anomalies
        </span>
      </div>
    </div>
  )
}
