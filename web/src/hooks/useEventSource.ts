import { useEffect, useReducer, useRef } from 'react'
import type { EventRow } from '@/lib/api'
import { reportFailure, reportSuccess } from '@/lib/connection'

export type SSEStatus = 'connecting' | 'open' | 'error'

export interface EventSourceState {
  events: EventRow[]
  status: SSEStatus
  reconnectCount: number
}

const MAX_BUFFER = 100
const MAX_RECONNECTS = 5
const BASE_DELAY_MS = 1_000

type Action =
  | { type: 'open' }
  | { type: 'event'; payload: EventRow }
  | { type: 'error' }
  | { type: 'reconnect' }   // only dispatched after the first connect fails
  | { type: 'connecting' }  // dispatched on the very first connect
  | { type: 'seed'; events: EventRow[] }

function reducer(state: EventSourceState, action: Action): EventSourceState {
  switch (action.type) {
    case 'open':
      return { ...state, status: 'open' }
    case 'event': {
      // Skip if the event is already in the buffer (race between SSE delivery and seed).
      if (state.events.some((e) => e.id === action.payload.id)) return state
      const next = [action.payload, ...state.events]
      return { ...state, events: next.slice(0, MAX_BUFFER) }
    }
    case 'error':
      return { ...state, status: 'error' }
    case 'connecting':
      return { ...state, status: 'connecting' }
    case 'reconnect':
      return { ...state, status: 'connecting', reconnectCount: state.reconnectCount + 1 }
    case 'seed': {
      // Prepend SSE-pushed events already in state over the seeded batch,
      // then deduplicate by id and cap the buffer.
      const merged = [...state.events, ...action.events]
      const seen = new Set<string>()
      const deduped = merged.filter((e) => {
        if (seen.has(e.id)) return false
        seen.add(e.id)
        return true
      })
      return { ...state, events: deduped.slice(0, MAX_BUFFER) }
    }
    default:
      return state
  }
}

const initialState: EventSourceState = {
  events: [],
  status: 'connecting',
  reconnectCount: 0,
}

/**
 * Subscribes to an SSE endpoint and accumulates events in a ring buffer.
 * Reconnects with exponential backoff on error (max 5 times).
 *
 * @param url Full SSE URL including auth query param. Pass null to disable.
 * @param seedEvents Optional initial events to populate the buffer before the stream opens.
 */
export function useEventSource(
  url: string | null,
  seedEvents: EventRow[] = [],
): EventSourceState {
  const [state, dispatch] = useReducer(reducer, initialState)
  const seeded = useRef(false)

  // Seed the buffer once from the initial REST fetch
  useEffect(() => {
    if (!seeded.current && seedEvents.length > 0) {
      seeded.current = true
      dispatch({ type: 'seed', events: seedEvents })
    }
  }, [seedEvents])

  useEffect(() => {
    if (url === null) return

    let es: EventSource | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let attempt = 0
    let cancelled = false

    const connect = (isReconnect = false) => {
      if (cancelled) return
      dispatch(isReconnect ? { type: 'reconnect' } : { type: 'connecting' })

      es = new EventSource(url)

      es.onopen = () => {
        if (cancelled) return
        attempt = 0
        dispatch({ type: 'open' })
        reportSuccess()
      }

      es.onmessage = (evt) => {
        if (cancelled) return
        try {
          const row = JSON.parse(evt.data as string) as EventRow
          dispatch({ type: 'event', payload: row })
        } catch {
          // malformed message — skip
        }
      }

      es.onerror = () => {
        if (cancelled) return
        es?.close()
        dispatch({ type: 'error' })
        reportFailure()

        if (attempt >= MAX_RECONNECTS) return

        const delay = BASE_DELAY_MS * 2 ** attempt
        attempt++
        reconnectTimer = setTimeout(() => connect(true), delay)
      }
    }

    connect()

    return () => {
      cancelled = true
      es?.close()
      if (reconnectTimer !== null) clearTimeout(reconnectTimer)
    }
  }, [url])

  return state
}
