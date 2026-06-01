import { useMemo } from 'react'
import { usePolledResource } from '@/hooks/usePolledResource'
import { getStats, listEvents, type StatsResult, type EventsResponse } from '@/lib/api'
import { formatEventName } from '@/lib/format'
import { Card, CardContent } from '@/components/ui/card'

const STATS_POLL_MS = 3_000
const FEED_POLL_MS = 5_000

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
        <div className="mt-1.5 truncate text-2xl font-bold tabular-nums">
          {typeof value === 'number' ? value.toLocaleString() : value}
        </div>
      </CardContent>
    </Card>
  )
}

export function StatsPanel() {
  const { data: stats, status: statsStatus } = usePolledResource<StatsResult>(getStats, STATS_POLL_MS)
  const { data: feed } = usePolledResource<EventsResponse>(
    () => listEvents({ limit: 200 }),
    FEED_POLL_MS,
  )

  const uniqueUsers = useMemo(() => {
    if (!feed?.events?.length) return null
    const ids = new Set(feed.events.map((e) => e.user_id).filter(Boolean))
    return ids.size
  }, [feed])

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-xs font-mono font-semibold uppercase tracking-widest text-muted-foreground">
          Stats
        </h2>
        {statsStatus === 'error' && (
          <span className="text-xs text-amber-500" role="status" aria-label="stale">
            stale
          </span>
        )}
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label="Total events" value={stats?.total_events ?? '—'} />
        <StatCard label="Today" value={stats?.today_count ?? '—'} />
        <StatCard label="Top event" value={stats?.top_events[0]?.event ? formatEventName(stats.top_events[0].event) : '—'} />
        <StatCard label="Unique users" value={uniqueUsers ?? '—'} />
      </div>
    </div>
  )
}
