/**
 * components/RdsBackupStatus.tsx
 *
 * RDS Backup Health Panel — a pure status/metric display component.
 *
 * Design contract:
 * ────────────────────────────────────────────────────────────────────
 *  • Purely declarative — no canvas, no charts. This data is state-based
 *    (availability, lag, snapshot count) not time-series, so a clean
 *    status-grid layout is the right affordance.
 *  • Uses only Tailwind utility classes from the project design system.
 *  • Renders a "degraded" error state when probe_error is set.
 *  • backup_lag_seconds > 300 triggers an amber alert indicator.
 *  • Null metrics (probe not configured) renders a clean placeholder.
 * ────────────────────────────────────────────────────────────────────
 */
import React from 'react'
import type { RDSMetrics } from '../types/telemetry'

// ─── Constants ───────────────────────────────────────────────────────────────

/** Backup lag threshold in seconds. Aurora's PITR typically lags < 60s. */
const LAG_WARN_SECONDS  = 300    // 5 min — amber warning
const LAG_CRIT_SECONDS  = 600    // 10 min — red critical

// ─── Status helpers ───────────────────────────────────────────────────────────

function statusDot(status: string): React.ReactElement {
  const normalised = status.toLowerCase()
  if (normalised === 'available') {
    return <span className="inline-block w-1.5 h-1.5 rounded-full bg-emerald-400 mr-1.5 shrink-0" />
  }
  if (normalised.includes('backup') || normalised.includes('modif')) {
    return <span className="inline-block w-1.5 h-1.5 rounded-full bg-amber-400 mr-1.5 animate-pulse shrink-0" />
  }
  return <span className="inline-block w-1.5 h-1.5 rounded-full bg-red-500 mr-1.5 animate-pulse shrink-0" />
}

function lagColor(lagSeconds: number): string {
  if (lagSeconds >= LAG_CRIT_SECONDS) return 'text-red-400'
  if (lagSeconds >= LAG_WARN_SECONDS)  return 'text-amber-400'
  return 'text-emerald-400'
}

function formatLag(lagSeconds: number): string {
  if (lagSeconds < 60) return `${Math.round(lagSeconds)}s`
  const m = Math.floor(lagSeconds / 60)
  const s = Math.round(lagSeconds % 60)
  return `${m}m ${s}s`
}

function formatTimestamp(iso: string): string {
  if (!iso || iso.startsWith('0001')) return '—'
  try {
    const d = new Date(iso)
    return d.toLocaleTimeString('en-GB', { hour12: false }) +
           ' ' +
           d.toLocaleDateString('en-GB', { day: '2-digit', month: 'short' })
  } catch {
    return '—'
  }
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function MetricRow({
  label,
  value,
  valueClass = 'text-zinc-200',
}: {
  label: string
  value: React.ReactNode
  valueClass?: string
}) {
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-zinc-900 last:border-0">
      <span className="text-[10px] uppercase tracking-widest text-zinc-600 font-mono">{label}</span>
      <span className={`text-[11px] font-mono font-semibold ${valueClass}`}>{value}</span>
    </div>
  )
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-[9px] uppercase tracking-widest text-zinc-700 font-mono mb-2 mt-4 first:mt-0">
      {children}
    </div>
  )
}

// ─── Main Component ──────────────────────────────────────────────────────────

interface Props {
  metrics: RDSMetrics | null
}

export function RdsBackupStatus({ metrics }: Props) {

  // ── Not configured ────────────────────────────────────────────────────────
  if (!metrics) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-2 py-8">
        <div className="w-2 h-2 rounded-full bg-zinc-700" />
        <p className="text-[10px] uppercase tracking-widest text-zinc-600 font-mono text-center">
          RDS probe not configured
        </p>
        <p className="text-[9px] text-zinc-700 font-mono text-center leading-relaxed">
          Set <code className="text-zinc-500">RDS_CLUSTER_ID</code> env var to enable
        </p>
      </div>
    )
  }

  // ── Probe error state ──────────────────────────────────────────────────────
  if (metrics.probe_error) {
    return (
      <div className="flex flex-col gap-2 p-3 border border-red-900 rounded-sm bg-red-950/20">
        <div className="flex items-center gap-2">
          <span className="w-1.5 h-1.5 rounded-full bg-red-500 animate-pulse shrink-0" />
          <span className="text-[10px] uppercase tracking-widest text-red-400 font-mono font-bold">
            Probe Error
          </span>
        </div>
        <p className="text-[9px] text-red-400/80 font-mono leading-relaxed break-all">
          {metrics.probe_error}
        </p>
      </div>
    )
  }

  const lagSec = metrics.backup_lag_seconds ?? 0

  return (
    <div className="flex flex-col gap-0 overflow-y-auto">

      {/* ── Cluster Identity ──────────────────────────────────────── */}
      <SectionLabel>Cluster</SectionLabel>
      <MetricRow
        label="ID"
        value={
          <span className="flex items-center">
            {statusDot(metrics.status)}
            {metrics.cluster_id}
          </span>
        }
      />
      <MetricRow label="Engine"   value={`${metrics.engine} ${metrics.engine_version}`} />
      <MetricRow label="Status"   value={metrics.status} valueClass={
        metrics.status === 'available' ? 'text-emerald-400' : 'text-amber-400'
      } />
      <MetricRow label="Multi-AZ" value={metrics.multi_az ? 'Yes' : 'No'} valueClass={
        metrics.multi_az ? 'text-emerald-400' : 'text-zinc-500'
      } />

      {/* ── Continuous Backup / PITR ──────────────────────────────── */}
      <SectionLabel>Point-in-Time Recovery</SectionLabel>
      <MetricRow
        label="Backup Lag"
        value={formatLag(lagSec)}
        valueClass={lagColor(lagSec)}
      />
      {lagSec >= LAG_WARN_SECONDS && (
        <div className="flex items-center gap-2 py-1.5 mb-1">
          <span className="w-1.5 h-1.5 rounded-full bg-amber-400 animate-pulse shrink-0" />
          <span className="text-[9px] text-amber-400 font-mono uppercase tracking-widest">
            Backup lag exceeds {LAG_WARN_SECONDS / 60}m threshold
          </span>
        </div>
      )}
      <MetricRow label="Retention"  value={`${metrics.backup_retention_days}d`} />
      <MetricRow label="Window"     value={metrics.preferred_backup_window || '—'} />
      <MetricRow label="PITR Start" value={formatTimestamp(metrics.earliest_restorable_time)} />
      <MetricRow label="PITR End"   value={formatTimestamp(metrics.latest_restorable_time)} />

      {/* ── Snapshot Inventory ────────────────────────────────────── */}
      <SectionLabel>Automated Snapshots</SectionLabel>
      <MetricRow label="Total"   value={metrics.automated_snapshot_count} />
      <MetricRow label="Complete" value={metrics.completed_snapshot_count} valueClass="text-emerald-400" />
      <MetricRow
        label="Latest"
        value={formatTimestamp(metrics.latest_snapshot_time)}
        valueClass={metrics.latest_snapshot_status === 'available' ? 'text-zinc-200' : 'text-amber-400'}
      />
      <MetricRow label="Size" value={`${metrics.latest_snapshot_size_gb} GiB`} />
      <MetricRow
        label="Encrypted"
        value={metrics.latest_snapshot_encrypted ? 'Yes ✓' : 'No ✗'}
        valueClass={metrics.latest_snapshot_encrypted ? 'text-emerald-400' : 'text-red-400'}
      />

      {/* Snapshot query error (non-fatal) */}
      {metrics.snapshot_query_error && (
        <div className="mt-2 p-2 border border-zinc-800 rounded-sm">
          <p className="text-[9px] text-amber-500/80 font-mono leading-relaxed break-all">
            ⚠ Snapshot query: {metrics.snapshot_query_error}
          </p>
        </div>
      )}
    </div>
  )
}
