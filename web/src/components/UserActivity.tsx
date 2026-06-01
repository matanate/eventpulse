import { useMemo, useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { usePolledResource } from '@/hooks/usePolledResource'
import { listEvents, listUserEvents, type EventRow, type EventsResponse } from '@/lib/api'

const POLL_MS = 12_000

function timeAgo(iso: string): string {
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (diff < 60) return `${diff}s`
  if (diff < 3_600) return `${Math.floor(diff / 60)}m`
  return `${Math.floor(diff / 3_600)}h`
}

export function UserActivity() {
  const { data: feedData } = usePolledResource<EventsResponse>(
    () => listEvents({ limit: 100 }),
    POLL_MS,
  )

  const [selectedUser, setSelectedUser] = useState<string | null>(null)
  const [userEvents, setUserEvents] = useState<EventRow[]>([])
  const [loading, setLoading] = useState(false)

  const topUsers = useMemo(() => {
    if (!feedData?.events?.length) return []
    const counts: Record<string, number> = {}
    for (const evt of feedData.events) {
      if (evt.user_id) counts[evt.user_id] = (counts[evt.user_id] ?? 0) + 1
    }
    return Object.entries(counts)
      .sort(([, a], [, b]) => b - a)
      .slice(0, 6)
      .map(([userId, count]) => ({ userId, count }))
  }, [feedData])

  async function handleSelectUser(userId: string) {
    if (selectedUser === userId) {
      setSelectedUser(null)
      setUserEvents([])
      return
    }
    setSelectedUser(userId)
    setLoading(true)
    try {
      const events = await listUserEvents(userId, 8)
      setUserEvents(events)
    } catch {
      setUserEvents([])
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
          User Activity
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {topUsers.length === 0 ? (
          <p className="py-2 text-center text-xs text-muted-foreground">
            No users yet — send some events
          </p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {topUsers.map(({ userId, count }) => (
              <button
                key={userId}
                type="button"
                onClick={() => { void handleSelectUser(userId) }}
                className={cn(
                  'rounded-full px-2.5 py-0.5 text-xs font-mono transition-all border',
                  selectedUser === userId
                    ? 'border-primary bg-primary/15 text-primary'
                    : 'border-border text-muted-foreground hover:border-primary/50 hover:text-foreground',
                )}
              >
                {userId}
                <span className="ml-1 opacity-60">×{count}</span>
              </button>
            ))}
          </div>
        )}

        {selectedUser && (
          <>
            <Separator />
            <div className="space-y-1">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground/50">
                {selectedUser} — recent events
              </p>
              {loading ? (
                <p className="py-2 text-center text-xs text-muted-foreground">Loading…</p>
              ) : userEvents.length === 0 ? (
                <p className="py-2 text-center text-xs text-muted-foreground">No events found</p>
              ) : (
                <ScrollArea className="h-32">
                  <div className="space-y-1 pr-3">
                    {userEvents.map((evt) => (
                      <div
                        key={evt.id}
                        className="flex items-center justify-between gap-2 rounded px-2 py-1 bg-secondary/30"
                      >
                        <span className="font-mono text-[11px] text-primary/80 truncate">
                          {evt.event}
                        </span>
                        <span className="text-[10px] text-muted-foreground/50 tabular-nums shrink-0">
                          {timeAgo(evt.timestamp)} ago
                        </span>
                      </div>
                    ))}
                  </div>
                </ScrollArea>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
