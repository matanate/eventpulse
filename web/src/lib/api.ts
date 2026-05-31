import { API_BASE_URL, DEMO_PROJECT_ID, DEMO_API_KEY } from './constants'

export interface EventCount {
  event: string
  count: number
}

export interface StatsResult {
  total_events: number
  today_count: number
  top_events: EventCount[]
}

export interface EventRow {
  id: string
  event: string
  user_id?: string
  properties?: Record<string, unknown>
  timestamp: string
  received_at: string
}

export interface EventsResponse {
  events: EventRow[]
  total: number
  limit: number
  offset: number
}

export interface TopEventsResponse {
  events: EventCount[]
  n: number
}

export interface PostEventPayload {
  event: string
  user_id: string
  properties?: Record<string, unknown>
}

export type PostEventResult =
  | { ok: true }
  | { ok: false; status: 429; retryAfter: number }
  | { ok: false; status: number; message: string }

function authHeaders(): HeadersInit {
  return {
    Authorization: `Bearer ${DEMO_API_KEY}`,
    'Content-Type': 'application/json',
  }
}

export async function postEvent(payload: PostEventPayload): Promise<PostEventResult> {
  const res = await fetch(`${API_BASE_URL}/v1/events`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify(payload),
  })

  if (res.status === 202) return { ok: true }

  if (res.status === 429) {
    const retryAfter = parseInt(res.headers.get('Retry-After') ?? '60', 10)
    return { ok: false, status: 429, retryAfter }
  }

  return { ok: false, status: res.status, message: `HTTP ${res.status}` }
}

export async function getStats(): Promise<StatsResult> {
  const res = await fetch(
    `${API_BASE_URL}/v1/projects/${DEMO_PROJECT_ID}/stats`,
    { headers: authHeaders() },
  )
  if (!res.ok) throw new Error(`stats: HTTP ${res.status}`)
  return res.json() as Promise<StatsResult>
}

export async function listEvents(limit = 20): Promise<EventRow[]> {
  const res = await fetch(
    `${API_BASE_URL}/v1/projects/${DEMO_PROJECT_ID}/events?limit=${limit}`,
    { headers: authHeaders() },
  )
  if (!res.ok) throw new Error(`events: HTTP ${res.status}`)
  const data = (await res.json()) as EventsResponse
  return data.events
}

export async function getTopEvents(n = 5): Promise<EventCount[]> {
  const res = await fetch(
    `${API_BASE_URL}/v1/projects/${DEMO_PROJECT_ID}/events/top?n=${n}`,
    { headers: authHeaders() },
  )
  if (!res.ok) throw new Error(`top events: HTTP ${res.status}`)
  const data = (await res.json()) as TopEventsResponse
  return data.events
}
