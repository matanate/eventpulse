import { useEffect, useState } from 'react'
import { listEvents, type EventRow } from '../lib/api'

const POLL_MS = 3_000

function timeAgo(iso: string): string {
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (diff < 60) return `${diff}s ago`
  if (diff < 3_600) return `${Math.floor(diff / 60)}m ago`
  return `${Math.floor(diff / 3_600)}h ago`
}

function EventItem({ evt }: { evt: EventRow }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-2.5">
      <div className="flex items-center gap-2 min-w-0">
        <span className="flex-shrink-0 h-1.5 w-1.5 rounded-full bg-cyan-400" />
        <span className="font-mono text-xs text-cyan-300 truncate">{evt.event}</span>
        {evt.user_id && (
          <span className="text-xs text-zinc-500 truncate">{evt.user_id}</span>
        )}
      </div>
      <span className="flex-shrink-0 text-xs tabular-nums text-zinc-600">
        {timeAgo(evt.timestamp)}
      </span>
    </div>
  )
}

export function EventFeed() {
  const [events, setEvents] = useState<EventRow[]>([])

  useEffect(() => {
    let cancelled = false

    const poll = async () => {
      try {
        const data = await listEvents(20)
        if (!cancelled) setEvents(data)
      } catch {
        // preserve last known state
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
        Live Feed
      </h2>
      <div className="space-y-1.5 max-h-64 overflow-y-auto pr-1">
        {events.length === 0 ? (
          <p className="py-4 text-center text-xs text-zinc-600">
            No events yet — send one on the left
          </p>
        ) : (
          events.map((evt) => <EventItem key={evt.id} evt={evt} />)
        )}
      </div>
    </div>
  )
}
