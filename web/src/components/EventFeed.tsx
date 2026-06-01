import { useCallback, useEffect, useRef, useState } from 'react'
import { usePolledResource } from '../hooks/usePolledResource'
import { listEvents, type EventRow, type EventsResponse } from '../lib/api'
import { EventFilters } from './EventFilters'

const POLL_MS = 3_000

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
  const hasProperties =
    evt.properties != null && Object.keys(evt.properties).length > 0

  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900/40">
      <div className="flex items-center justify-between gap-3 px-3 py-2.5">
        <div className="flex min-w-0 items-center gap-2">
          <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-cyan-400" />
          <span className="truncate font-mono text-xs text-cyan-300">{evt.event}</span>
          {evt.user_id && (
            <button
              type="button"
              onClick={() => onFilterByUser(evt.user_id!)}
              className="truncate text-xs text-zinc-500 transition-colors hover:text-zinc-300"
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
              className="rounded px-1.5 py-0.5 text-xs text-zinc-600 transition-colors hover:bg-zinc-800 hover:text-zinc-400"
            >
              {expanded ? '▲' : '≡'}
            </button>
          )}
          <span className="tabular-nums text-xs text-zinc-600">
            {timeAgo(evt.timestamp)}
          </span>
        </div>
      </div>

      {expanded && hasProperties && (
        <div className="border-t border-zinc-800 px-3 py-2">
          <div className="rounded bg-zinc-800/60 px-2 py-1.5 font-mono text-xs text-zinc-400">
            {Object.entries(evt.properties!).map(([k, v]) => (
              <div key={k} className="flex gap-2">
                <span className="text-cyan-400/60">{k}:</span>
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

  const fetcher = useCallback(
    () => listEvents({ limit: 20, event: eventFilter || undefined, user_id: userIdFilter || undefined }),
    [eventFilter, userIdFilter],
  )

  const { data: response, status, refetch } = usePolledResource<EventsResponse>(fetcher, POLL_MS)

  // Re-poll immediately when filters change (skip the initial mount)
  const isFirstFilterRender = useRef(true)
  useEffect(() => {
    if (isFirstFilterRender.current) {
      isFirstFilterRender.current = false
      return
    }
    refetch()
  }, [eventFilter, userIdFilter, refetch])

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

  const events = response?.events ?? []
  const total = response?.total ?? 0

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-widest text-zinc-400">
          Live Feed
        </h2>
        <div className="flex items-center gap-2">
          {total > 0 && (
            <span className="text-xs tabular-nums text-zinc-600">
              {events.length}/{total}
            </span>
          )}
          {status === 'error' && (
            <span className="text-xs text-amber-500" role="status" aria-label="stale">
              stale
            </span>
          )}
        </div>
      </div>

      <EventFilters
        eventFilter={eventFilter}
        userIdFilter={userIdFilter}
        onEventFilterChange={setEventFilter}
        onUserIdFilterChange={setUserIdFilter}
      />

      <div className="max-h-72 space-y-1.5 overflow-y-auto pr-1">
        {events.length === 0 ? (
          <p className="py-4 text-center text-xs text-zinc-600">
            {eventFilter || userIdFilter
              ? 'No events match the current filters'
              : 'No events yet — send one on the left'}
          </p>
        ) : (
          events.map((evt) => (
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
    </div>
  )
}
