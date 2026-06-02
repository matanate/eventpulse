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

export interface ListEventsOptions {
  limit?: number
  offset?: number
  event?: string
  user_id?: string
  from?: string // ISO 8601 timestamp — filter events at or after this time
}

export interface BatchResult {
  count: number
  status: string
}

export type PostEventResult =
  | { ok: true }
  | { ok: false; status: 429; retryAfter: number }
  | { ok: false; status: number; message: string }

export type PostBatchResult =
  | { ok: true; count: number }
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

export type DuplicateResult = { first: PostEventResult; second: PostEventResult }

export async function postEventDuplicate(payload: PostEventPayload): Promise<DuplicateResult> {
  const key = crypto.randomUUID()
  const send = async (): Promise<PostEventResult> => {
    const res = await fetch(`${API_BASE_URL}/v1/events`, {
      method: 'POST',
      headers: { ...authHeaders(), 'Idempotency-Key': key },
      body: JSON.stringify(payload),
    })
    if (res.status === 202) return { ok: true }
    if (res.status === 429) {
      const retryAfter = parseInt(res.headers.get('Retry-After') ?? '60', 10)
      return { ok: false, status: 429, retryAfter }
    }
    return { ok: false, status: res.status, message: `HTTP ${res.status}` }
  }
  const first = await send()
  const second = await send()
  return { first, second }
}

export async function postEventWithBadKey(payload: PostEventPayload): Promise<PostEventResult> {
  const res = await fetch(`${API_BASE_URL}/v1/events`, {
    method: 'POST',
    headers: {
      Authorization: 'Bearer invalid_key_demo',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  })

  if (res.status === 202) return { ok: true }

  if (res.status === 429) {
    const retryAfter = parseInt(res.headers.get('Retry-After') ?? '60', 10)
    return { ok: false, status: 429, retryAfter }
  }

  return { ok: false, status: res.status, message: `HTTP ${res.status}` }
}

export async function postEventsBatch(events: PostEventPayload[]): Promise<PostBatchResult> {
  const res = await fetch(`${API_BASE_URL}/v1/events/batch`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify({ events }),
  })

  if (res.status === 202) {
    const data = (await res.json()) as BatchResult
    return { ok: true, count: data.count }
  }

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

export async function listEvents(options: ListEventsOptions = {}): Promise<EventsResponse> {
  const { limit = 20, offset = 0, event, user_id, from } = options
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (event) params.set('event', event)
  if (user_id) params.set('user_id', user_id)
  if (from) params.set('from', from)

  const res = await fetch(
    `${API_BASE_URL}/v1/projects/${DEMO_PROJECT_ID}/events?${params.toString()}`,
    { headers: authHeaders() },
  )
  if (!res.ok) throw new Error(`events: HTTP ${res.status}`)
  return res.json() as Promise<EventsResponse>
}

export async function listUserEvents(userId: string, limit = 20): Promise<EventRow[]> {
  const params = new URLSearchParams({ limit: String(limit) })
  const res = await fetch(
    `${API_BASE_URL}/v1/projects/${DEMO_PROJECT_ID}/users/${encodeURIComponent(userId)}/events?${params.toString()}`,
    { headers: authHeaders() },
  )
  if (!res.ok) throw new Error(`user events: HTTP ${res.status}`)
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

export interface QueueStats {
  pending_messages: number
  dead_letter_count: number
}

export interface FunnelStep {
  event: string
  entered: number
  converted: number
  dropped: number
  conversion_rate: number // 0 for the last step
}

export interface FunnelResult {
  steps: FunnelStep[]
  window: string
  overall_conversion_rate: number
}

export async function postFunnel(steps: string[], windowPeriod: string): Promise<FunnelResult> {
  const res = await fetch(
    `${API_BASE_URL}/v1/projects/${DEMO_PROJECT_ID}/funnels`,
    {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ steps, window: windowPeriod }),
    },
  )
  if (!res.ok) throw new Error(`funnel: HTTP ${res.status}`)
  return res.json() as Promise<FunnelResult>
}

export interface RetentionBucket {
  offset: number
  count: number
  rate: number
}

export interface RetentionRow {
  cohort_date: string
  cohort_size: number
  buckets: RetentionBucket[]
}

export interface RetentionResult {
  period: string
  cohorts: number
  rows: RetentionRow[]
}

export async function getRetention(period = 'day', cohorts = 8): Promise<RetentionResult> {
  const params = new URLSearchParams({ period, cohorts: String(cohorts) })
  const res = await fetch(
    `${API_BASE_URL}/v1/projects/${DEMO_PROJECT_ID}/retention?${params.toString()}`,
    { headers: authHeaders() },
  )
  if (!res.ok) throw new Error(`retention: HTTP ${res.status}`)
  return res.json() as Promise<RetentionResult>
}

export async function getQueueStats(): Promise<QueueStats> {
  const res = await fetch(`${API_BASE_URL}/v1/admin/queue/stats`, {
    headers: authHeaders(),
  })
  if (!res.ok) throw new Error(`queue stats: HTTP ${res.status}`)
  return res.json() as Promise<QueueStats>
}

/**
 * Returns the SSE stream URL for a project's event feed.
 * The API key is passed as a query param because EventSource cannot set headers.
 */
export function sseEventsUrl(projectId: string): string {
  return `${API_BASE_URL}/v1/projects/${projectId}/stream?api_key=${encodeURIComponent(DEMO_API_KEY)}`
}
