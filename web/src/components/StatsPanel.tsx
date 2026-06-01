import { usePolledResource } from '../hooks/usePolledResource'
import { getStats, type StatsResult } from '../lib/api'

const POLL_MS = 3_000

interface StatCardProps {
  label: string
  value: number | string
}

function StatCard({ label, value }: StatCardProps) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
      <div className="text-xs font-medium uppercase tracking-widest text-zinc-500">
        {label}
      </div>
      <div className="mt-1.5 text-2xl font-bold tabular-nums text-zinc-100">
        {typeof value === 'number' ? value.toLocaleString() : value}
      </div>
    </div>
  )
}

export function StatsPanel() {
  const { data: stats, status } = usePolledResource<StatsResult>(getStats, POLL_MS)

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-widest text-zinc-400">
          Stats
        </h2>
        {status === 'error' && (
          <span className="text-xs text-amber-500" role="status" aria-label="stale">
            stale
          </span>
        )}
      </div>
      <div className="grid grid-cols-2 gap-3">
        <StatCard label="Total events" value={stats?.total_events ?? '—'} />
        <StatCard label="Today" value={stats?.today_count ?? '—'} />
      </div>
    </div>
  )
}
