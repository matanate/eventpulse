import { usePolledResource } from '@/hooks/usePolledResource'
import { getStats, type StatsResult } from '@/lib/api'
import { Card, CardContent } from '@/components/ui/card'

const POLL_MS = 3_000

interface StatCardProps {
  label: string
  value: number | string
}

function StatCard({ label, value }: StatCardProps) {
  return (
    <Card>
      <CardContent className="pt-4 pb-4">
        <div className="text-xs font-medium uppercase tracking-widest text-muted-foreground">
          {label}
        </div>
        <div className="mt-1.5 text-2xl font-bold tabular-nums">
          {typeof value === 'number' ? value.toLocaleString() : value}
        </div>
      </CardContent>
    </Card>
  )
}

export function StatsPanel() {
  const { data: stats, status } = usePolledResource<StatsResult>(getStats, POLL_MS)

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-xs font-mono font-semibold uppercase tracking-widest text-muted-foreground">
          Stats
        </h2>
        {status === 'error' && (
          <span className="text-xs text-amber-500" role="status" aria-label="stale">
            stale
          </span>
        )}
      </div>
      <div className="grid grid-cols-3 gap-3">
        <StatCard label="Total events" value={stats?.total_events ?? '—'} />
        <StatCard label="Today" value={stats?.today_count ?? '—'} />
        <StatCard label="Top event" value={stats?.top_events[0]?.event ?? '—'} />
      </div>
    </div>
  )
}
