/**
 * components/ChartLegend.tsx
 * Minimal legend displaying live metrics next to the chart.
 */
import { ENDPOINT_IDS, ENDPOINT_META } from '../lib/constants'
import type { EndpointStateMap } from '../types/telemetry'

export function ChartLegend({ endpoints }: { endpoints: EndpointStateMap }) {
  return (
    <div className="flex gap-6 mt-4">
      {ENDPOINT_IDS.map(id => {
        const meta = ENDPOINT_META[id]
        const state = endpoints.get(id)
        const lat = state?.latency_ms
        const displayLat = lat !== undefined ? lat.toFixed(1) : '---'

        return (
          <div key={id} className="flex items-center gap-2">
            <div
              className="w-2 h-2 rounded-full shrink-0"
              style={{ background: meta?.color ?? '#fff' }}
            />
            <span className="text-[11px] font-sans text-zinc-500 uppercase tracking-widest">
              {id}
            </span>
            <span className="text-xs font-mono font-bold text-zinc-300">
              {displayLat}
              <span className="text-[9px] text-zinc-600 font-normal"> ms</span>
            </span>
          </div>
        )
      })}
    </div>
  )
}
