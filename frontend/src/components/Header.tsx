/**
 * components/Header.tsx
 * Top application bar — minimalist all-black and chrome design.
 */
import type { ConnectionStatus } from '../types/telemetry'

interface HeaderProps {
  status: ConnectionStatus
  frameCount: number
  endpointCount: number
  onReconnect: () => void
}

const STATUS_LABEL: Record<ConnectionStatus, string> = {
  connecting:   'Connecting…',
  connected:    'Live',
  disconnected: 'Disconnected',
}

export function Header({ status, frameCount, endpointCount, onReconnect }: HeaderProps) {
  return (
    <header className="h-14 flex items-center px-5 gap-4 border-b border-zinc-900 bg-black relative z-10 shrink-0">
      {/* Logo / wordmark */}
      <div className="flex items-center gap-3 mr-2">
        {/* Minimal icon */}
        <div className="w-4 h-4 bg-white rounded-sm shrink-0" />
        <div>
          <div className="text-[15px] font-bold tracking-tight leading-none text-white font-sans">
            TelemetryPulse
          </div>
          <div className="text-[9px] text-zinc-500 tracking-widest uppercase leading-snug mt-0.5 font-sans">
            Network Observability Engine
          </div>
        </div>
      </div>

      {/* Gradient separator line */}
      <div className="w-px h-6 bg-zinc-900 shrink-0" />

      {/* Connection status badge */}
      <div className="flex items-center gap-2 text-xs font-mono">
        <span className={`w-1.5 h-1.5 rounded-full ${status === 'connected' ? 'bg-zinc-300' : 'bg-red-500'}`} />
        <span className={status === 'connected' ? 'text-zinc-300' : 'text-red-500'}>
          {STATUS_LABEL[status]}
        </span>
      </div>

      {/* Metrics */}
      <div className="flex gap-5 ml-1 text-[11px] text-zinc-500 font-mono">
        <span>
          <span className="text-zinc-600">endpoints&nbsp;</span>
          <span className="text-zinc-300 font-bold">{endpointCount}</span>
        </span>
        <span>
          <span className="text-zinc-600">rAF frames&nbsp;</span>
          <span className="text-zinc-300 font-bold">{frameCount.toLocaleString()}</span>
        </span>
      </div>

      {/* Spacer */}
      <div className="flex-1" />

      {/* Reconnect button */}
      {status === 'disconnected' && (
        <button 
          className="text-xs font-mono px-3 py-1 border border-zinc-800 text-zinc-400 hover:text-white hover:border-white transition-colors" 
          onClick={onReconnect}
        >
          ↺ Reconnect
        </button>
      )}

      {/* Timestamp */}
      <div className="text-[11px] font-mono text-zinc-600 tracking-wide">
        {new Date().toLocaleTimeString('en-GB', { hour12: false })} UTC+5:30
      </div>
    </header>
  )
}
