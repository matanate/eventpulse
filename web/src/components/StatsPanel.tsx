import { useEffect, useState } from 'react'
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
  const [stats, setStats] = useState<StatsResult | null>(null)

  useEffect(() => {
    let cancelled = false

    const poll = async () => {
      try {
        const data = await getStats()
        if (!cancelled) setStats(data)
      } catch {
        // preserve last known value
      }
    }

    poll()
    const id = setInterval(poll, POLL_MS)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])

  return (
    <div className="space-y-3">
      <h2 className="text-sm font-semibold uppercase tracking-widest text-zinc-400">
        Stats
      </h2>
      <div className="grid grid-cols-2 gap-3">
        <StatCard label="Total events" value={stats?.total_events ?? '—'} />
        <StatCard label="Today" value={stats?.today_count ?? '—'} />
      </div>
    </div>
  )
}
