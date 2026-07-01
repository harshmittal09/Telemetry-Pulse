/**
 * components/EndpointCard.tsx
 *
 * Phase 1 Refactor: Ultra-minimal "all-black and chrome" aesthetic.
 * Removes all bounding boxes and neon elements, replacing them with
 * stark typographic hierarchy and whitespace.
 */
import type { TelemetryPayload } from '../types/telemetry'
import { ANOMALY_THRESHOLD } from '../lib/constants'

interface EndpointCardProps {
  payload: TelemetryPayload | undefined
  endpointId: string
}

function MetricGroup({ label, value, unit, highlight }: {
  label: string
  value: string
  unit?: string
  highlight?: boolean
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="text-[10px] uppercase tracking-widest text-zinc-500 font-sans">
        {label}
      </div>
      <div className="flex items-baseline gap-1">
        <span className={`text-xl font-bold font-mono ${highlight ? 'text-red-500' : 'text-white'}`}>
          {value}
        </span>
        {unit && (
          <span className="text-xs text-zinc-600 font-sans">{unit}</span>
        )}
      </div>
    </div>
  )
}

export function EndpointCard({ payload, endpointId }: EndpointCardProps) {
  if (!payload) {
    return (
      <div className="py-6 border-b border-zinc-900 last:border-0 flex flex-col gap-4 opacity-50">
        <div className="flex items-center gap-3">
          <span className="text-sm font-semibold text-zinc-400 font-mono">{endpointId}</span>
          <span className="text-xs text-zinc-600">Waiting for data...</span>
        </div>
      </div>
    )
  }

  const { z_score, is_anomaly, latency_ms, jitter_ms, packet_loss, mean_ms, std_dev_ms, target } = payload
  const isWarn = Math.abs(z_score) >= ANOMALY_THRESHOLD * 0.75 && !is_anomaly

  return (
    <div className="py-6 border-b border-zinc-900 last:border-0 flex flex-col gap-5">
      {/* ── Header row ──────────────────────────────────── */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className={`text-sm font-bold font-mono ${is_anomaly ? 'text-red-500' : 'text-white'}`}>
            {endpointId}
          </span>
          <span className="text-xs font-mono text-zinc-500 truncate max-w-[140px]">
            {target}
          </span>
        </div>
        
        {/* Status Indicator */}
        <div className="flex items-center gap-2">
          {is_anomaly ? (
            <>
              <div className="w-1.5 h-1.5 rounded-full bg-red-500 animate-pulse" />
              <span className="text-[10px] uppercase tracking-widest text-red-500 font-bold font-sans">Anomaly</span>
            </>
          ) : isWarn ? (
            <>
              <div className="w-1.5 h-1.5 rounded-full bg-zinc-400" />
              <span className="text-[10px] uppercase tracking-widest text-zinc-400 font-sans">Elevated</span>
            </>
          ) : (
            <>
              <div className="w-1 h-1 rounded-full bg-zinc-600" />
              <span className="text-[10px] uppercase tracking-widest text-zinc-500 font-sans">Normal</span>
            </>
          )}
        </div>
      </div>

      {/* ── Metrics Grid ─────────────────────────────────── */}
      <div className="grid grid-cols-3 gap-y-6 gap-x-4">
        <MetricGroup 
          label="Latency" 
          value={latency_ms.toFixed(1)} 
          unit="ms" 
          highlight={is_anomaly} 
        />
        <MetricGroup 
          label="Z-Score" 
          value={`${z_score >= 0 ? '+' : ''}${z_score.toFixed(2)}`} 
          highlight={Math.abs(z_score) >= ANOMALY_THRESHOLD} 
        />
        <MetricGroup 
          label="Loss" 
          value={packet_loss.toFixed(0)} 
          unit="%" 
          highlight={packet_loss > 0} 
        />
        
        <MetricGroup 
          label="Mean" 
          value={mean_ms.toFixed(1)} 
          unit="ms" 
        />
        <MetricGroup 
          label="Std Dev" 
          value={std_dev_ms.toFixed(2)} 
          unit="ms" 
        />
        <MetricGroup 
          label="Jitter" 
          value={jitter_ms.toFixed(1)} 
          unit="ms" 
        />
      </div>
    </div>
  )
}
