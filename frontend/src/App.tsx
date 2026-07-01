/**
 * App.tsx
 *
 * Root layout container for TelemetryPulse.
 * Implements a strict all-black and chrome minimalist design system.
 */
import { useEffect, useState, useMemo } from 'react'
import { useTelemetryStream }  from './hooks/useTelemetryStream'
import { Header }              from './components/Header'
import { EndpointCard }        from './components/EndpointCard'
import { LatencyChart }        from './components/LatencyChart'
import { ChartLegend }         from './components/ChartLegend'
import { VirtualLog }          from './components/VirtualLog'
import { AdminPanel }          from './components/AdminPanel'
import { ENDPOINT_IDS }        from './lib/constants'

// ─── Anomaly stats banner ────────────────────────────────────────────────
function AnomalyBanner({ anomalyCount, total }: { anomalyCount: number; total: number }) {
  if (anomalyCount === 0) return null
  return (
    <div className="flex items-center gap-3 py-2 border-b border-zinc-900">
      <div className="w-1.5 h-1.5 bg-red-500 rounded-full animate-pulse shrink-0" />
      <span className="text-[10px] uppercase tracking-widest text-red-500 font-bold font-sans">
        {anomalyCount} of {total} endpoints in anomaly state
      </span>
    </div>
  )
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-[10px] uppercase tracking-widest text-zinc-500 font-sans border-b border-zinc-900 pb-2 mb-4">
      {children}
    </div>
  )
}

export default function App() {
  const { endpoints, log, status: connectionStatus, frameCount, reconnect } = useTelemetryStream()
  const [mounted, setMounted] = useState(false)

  useEffect(() => { setMounted(true) }, [])

  // Calculate live anomaly stats across the 3 endpoints.
  const anomalyCount = useMemo(() => {
    let count = 0
    endpoints.forEach(ep => { if (ep.is_anomaly) count++ })
    return count
  }, [endpoints])

  if (!mounted) return null

  return (
    <div className="flex flex-col h-screen bg-black text-white font-sans overflow-hidden">
      
      {/* ── TOP HEADER ───────────────────────────────────────────────── */}
      <Header
        status={connectionStatus}
        frameCount={frameCount}
        endpointCount={endpoints.size}
        onReconnect={reconnect}
      />

      {/* ── MAIN CONTENT GRID ────────────────────────────────────────── */}
      <main className="flex-1 grid grid-cols-[380px_1fr] overflow-hidden">

        {/* ── LEFT SIDEBAR: Endpoint Cards ───────────────────────────── */}
        <aside className="border-r border-zinc-900 overflow-y-auto px-6 py-6 flex flex-col bg-black">
          <SectionHeading>Endpoints</SectionHeading>

          {/* Anomaly alert banner */}
          <AnomalyBanner anomalyCount={anomalyCount} total={ENDPOINT_IDS.length} />

          {/* Render cards in a deterministic order */}
          {ENDPOINT_IDS.map(id => (
            <EndpointCard
              key={id}
              endpointId={id}
              payload={endpoints.get(id)}
            />
          ))}

          {/* Phase 5: Failure Simulation Injection */}
          <div className="mt-8">
            <AdminPanel />
          </div>

          {/* System stats footer */}
          <div className="mt-auto pt-6 border-t border-zinc-900 text-[10px] text-zinc-500 font-mono leading-relaxed tracking-wider uppercase">
            <div>
              <span className="text-zinc-600">window</span> N=50 · O(1)
            </div>
            <div>
              <span className="text-zinc-600">threshold</span> |Z| ≥ 2.5σ
            </div>
            <div>
              <span className="text-zinc-600">probe</span> 500ms · log-normal
            </div>
            <div>
              <span className="text-zinc-600">transport</span> WebSocket → Redis
            </div>
          </div>
        </aside>

        {/* ── RIGHT COLUMN: Chart + Log ──────────────────────────────── */}
        <section className="flex flex-col min-w-0 min-h-0 overflow-hidden bg-black h-full">
          
          {/* Top Half: Real-time Canvas Chart */}
          <div className="h-[55%] flex flex-col border-b border-zinc-900 p-6 relative">
            <div className="flex justify-between items-end mb-4">
              <SectionHeading>Live Latency &amp; Anomalies</SectionHeading>
              <ChartLegend endpoints={endpoints} />
            </div>
            
            <div className="flex-1 relative min-h-0 w-full">
              <LatencyChart endpoints={endpoints} />
            </div>
          </div>

          {/* Bottom Half: Virtualized Event Log */}
          <div className="h-[45%] flex flex-col bg-black min-h-0">
            <VirtualLog log={log} />
          </div>

        </section>
      </main>
    </div>
  )
}
