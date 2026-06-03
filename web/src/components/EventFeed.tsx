import { useCallback, useMemo, useState } from 'react'
import { useEventSource } from '@/hooks/useEventSource'
import { usePolledResource } from '@/hooks/usePolledResource'
import { listEvents, sseEventsUrl, type EventRow, type EventsResponse } from '@/lib/api'
import { DEMO_PROJECT_ID } from '@/lib/constants'
import { formatEventName } from '@/lib/format'
import { eventColor } from '@/lib/eventColors'
import { EventFilters } from './EventFilters'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'

const INITIAL_LIMIT = 50

function timeAgo(iso: string): string {
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (diff < 60) return `${diff}s ago`
  if (diff < 3_600) return `${Math.floor(diff / 60)}m ago`
  return `${Math.floor(diff / 3_600)}h ago`
}

interface EventItemProps {
  evt: EventRow
  expanded: boolean
  onToggleExpand: () => void
  onFilterByUser: (userId: string) => void
}

function EventItem({ evt, expanded, onToggleExpand, onFilterByUser }: EventItemProps) {
  const hasProperties = evt.properties != null && Object.keys(evt.properties).length > 0

  return (
    <div className="rounded-lg border border-border bg-card/40">
      <div className="flex items-center justify-between gap-3 px-3 py-2.5">
        <div className="flex min-w-0 items-center gap-2">
          <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
          <Badge
            variant="outline"
            style={{ color: eventColor(evt.event), borderColor: `${eventColor(evt.event)}40`, backgroundColor: `${eventColor(evt.event)}0d` }}
            className="font-mono text-[10px] shrink-0 truncate max-w-[140px]"
          >
            {formatEventName(evt.event)}
          </Badge>
          {evt.user_id && (
            <button
              type="button"
              onClick={() => onFilterByUser(evt.user_id!)}
              className="truncate text-xs text-muted-foreground transition-colors hover:text-foreground"
              title={`Filter by ${evt.user_id}`}
            >
              {evt.user_id}
            </button>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {hasProperties && (
            <button
              type="button"
              onClick={onToggleExpand}
              aria-expanded={expanded}
              aria-label="Toggle properties"
              className="rounded px-1.5 py-0.5 text-xs text-muted-foreground/50 transition-colors hover:bg-secondary hover:text-muted-foreground"
            >
              {expanded ? '▲' : '≡'}
            </button>
          )}
          <span className="tabular-nums text-xs text-muted-foreground/50">
            {timeAgo(evt.timestamp)}
          </span>
        </div>
      </div>

      {expanded && hasProperties && (
        <div className="border-t border-border px-3 py-2">
          <div className="rounded bg-secondary/60 px-2 py-1.5 font-mono text-xs text-muted-foreground">
            {Object.entries(evt.properties!).map(([k, v]) => (
              <div key={k} className="flex gap-2">
                <span className="text-primary/60">{k}:</span>
                <span className="break-all">
                  {typeof v === 'object' ? JSON.stringify(v) : String(v)}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

export function EventFeed() {
  const [eventFilter, setEventFilter] = useState('')
  const [userIdFilter, setUserIdFilter] = useState('')
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())

  // Fetch initial events once on mount to seed the SSE ring buffer.
  const initialFetcher = useCallback(
    () => listEvents({ limit: INITIAL_LIMIT }),
    [],
  )
  const { data: initialData } = usePolledResource<EventsResponse>(initialFetcher, 0)
  const seedEvents = useMemo(() => initialData?.events ?? [], [initialData])

  const sseUrl = sseEventsUrl(DEMO_PROJECT_ID)
  const { events: allEvents, status } = useEventSource(sseUrl, seedEvents)

  // Client-side filtering of the ring buffer.
  const events = useMemo(
    () =>
      allEvents.filter(
        (e) =>
          (!eventFilter || e.event.toLowerCase().includes(eventFilter.toLowerCase())) &&
          (!userIdFilter || (e.user_id ?? '').toLowerCase().includes(userIdFilter.toLowerCase())),
      ),
    [allEvents, eventFilter, userIdFilter],
  )

  const handleToggleExpand = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const handleFilterByUser = useCallback((userId: string) => {
    setUserIdFilter(userId)
  }, [])

  // Poll fallback — only when SSE is in error state so the feed stays alive.
  const fallbackFetcher = useCallback(
    () => listEvents({ limit: 20, event: eventFilter || undefined, user_id: userIdFilter || undefined }),
    [eventFilter, userIdFilter],
  )
  const fallbackActive = status === 'error'
  const { data: fallbackData } = usePolledResource<EventsResponse>(
    fallbackFetcher,
    fallbackActive ? 3_000 : 0,
  )

  const displayEvents = fallbackActive && fallbackData
    ? fallbackData.events.filter(
        (e) =>
          (!eventFilter || e.event.toLowerCase().includes(eventFilter.toLowerCase())) &&
          (!userIdFilter || (e.user_id ?? '').toLowerCase().includes(userIdFilter.toLowerCase())),
      )
    : events

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            Live Feed
          </CardTitle>
          <div className="flex items-center gap-2">
            {displayEvents.length > 0 && (
              <span className="text-xs tabular-nums text-muted-foreground/50">
                {displayEvents.length}
              </span>
            )}
            {status === 'open' && (
              <span className="flex items-center gap-1 text-[10px] font-mono text-emerald-500" role="status" aria-label="SSE connected">
                <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                SSE
              </span>
            )}
            {fallbackActive && (
              <span className="flex items-center gap-1 text-[10px] font-mono text-amber-400" role="status" aria-label="polling fallback">
                <span className="h-1.5 w-1.5 rounded-full bg-amber-400" />
                polling
              </span>
            )}
            {status === 'connecting' && (
              <span className="flex items-center gap-1 text-[10px] font-mono text-muted-foreground/50" aria-label="connecting">
                <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50 animate-pulse" />
                connecting
              </span>
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <EventFilters
          eventFilter={eventFilter}
          userIdFilter={userIdFilter}
          onEventFilterChange={setEventFilter}
          onUserIdFilterChange={setUserIdFilter}
        />

        <ScrollArea className="h-72">
          <div className="space-y-1.5 pr-3">
            {displayEvents.length === 0 ? (
              <p className="py-4 text-center text-xs text-muted-foreground">
                {eventFilter || userIdFilter
                  ? 'No events match the current filters'
                  : 'No events yet — send one on the left'}
              </p>
            ) : (
              displayEvents.map((evt) => (
                <EventItem
                  key={evt.id}
                  evt={evt}
                  expanded={expandedIds.has(evt.id)}
                  onToggleExpand={() => handleToggleExpand(evt.id)}
                  onFilterByUser={handleFilterByUser}
                />
              ))
            )}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  )
}
