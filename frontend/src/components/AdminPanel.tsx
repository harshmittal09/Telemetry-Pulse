/**
 * components/AdminPanel.tsx
 *
 * Phase 1 Refactor: All-black and chrome minimalist design.
 */
import { useState } from 'react'

export function AdminPanel() {
  const [latencyMs, setLatencyMs] = useState<number>(2500)
  const [packetLoss, setPacketLoss] = useState<number>(0)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [lastResult, setLastResult] = useState<'success' | 'error' | null>(null)

  const handleInject = async () => {
    setIsSubmitting(true)
    setLastResult(null)

    try {
      const API_URL = import.meta.env.VITE_API_URL || '/api/simulate'
      const res = await fetch(API_URL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          latency_ms: Number(latencyMs),
          packet_loss: Number(packetLoss),
        }),
      })

      if (res.ok) {
        setLastResult('success')
        setTimeout(() => setLastResult(null), 3000)
      } else {
        setLastResult('error')
      }
    } catch (err) {
      console.error('Failed to inject anomaly:', err)
      setLastResult('error')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="text-[10px] uppercase tracking-widest text-zinc-500 font-sans border-b border-zinc-900 pb-2">
        Failure Injection
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">
            Latency (ms)
          </label>
          <input
            type="number"
            className="w-full bg-black border-b border-zinc-800 text-white font-mono text-sm py-1 focus:outline-none focus:border-white transition-colors"
            value={latencyMs}
            onChange={e => setLatencyMs(Number(e.target.value))}
            min={0}
            step={100}
          />
        </div>
        <div>
          <label className="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">
            Loss (%)
          </label>
          <input
            type="number"
            className="w-full bg-black border-b border-zinc-800 text-white font-mono text-sm py-1 focus:outline-none focus:border-white transition-colors"
            value={packetLoss}
            onChange={e => setPacketLoss(Number(e.target.value))}
            min={0}
            max={100}
          />
        </div>
      </div>

      <button
        className="w-full py-2 text-xs font-bold uppercase tracking-widest text-white border border-zinc-800 hover:bg-white hover:text-black transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        onClick={handleInject}
        disabled={isSubmitting}
      >
        {isSubmitting ? 'Injecting...' : 'Inject Spike'}
      </button>

      {lastResult === 'success' && (
        <div className="text-[10px] uppercase tracking-widest text-zinc-400 text-center font-sans">
          Spike Armed
        </div>
      )}
      {lastResult === 'error' && (
        <div className="text-[10px] uppercase tracking-widest text-red-500 text-center font-sans">
          Injection Failed
        </div>
      )}
    </div>
  )
}
