import { describe, it, expect } from 'vitest'
import { http, HttpResponse } from 'msw'
import { server } from '../test/server'
import {
  postEvent,
  postEventsBatch,
  getStats,
  listEvents,
  listUserEvents,
  getTopEvents,
} from './api'

const BASE = 'http://test.local'
const PROJECT_ID = 'test-project-id'

describe('postEvent', () => {
  it('returns ok on 202', async () => {
    const result = await postEvent({ event: 'page_viewed', user_id: 'user_1' })
    expect(result.ok).toBe(true)
  })

  it('returns 429 with retryAfter from header', async () => {
    server.use(
      http.post(`${BASE}/v1/events`, () =>
        HttpResponse.json(
          { error: 'rate_limited' },
          { status: 429, headers: { 'Retry-After': '30' } },
        ),
      ),
    )
    const result = await postEvent({ event: 'page_viewed', user_id: 'user_1' })
    expect(result.ok).toBe(false)
    expect(result.status).toBe(429)
    if (!result.ok && result.status === 429) {
      expect(result.retryAfter).toBe(30)
    }
  })

  it('defaults retryAfter to 60 when header is missing', async () => {
    server.use(
      http.post(`${BASE}/v1/events`, () =>
        HttpResponse.json({ error: 'rate_limited' }, { status: 429 }),
      ),
    )
    const result = await postEvent({ event: 'page_viewed', user_id: 'user_1' })
    expect(result.ok).toBe(false)
    if (!result.ok && result.status === 429) {
      expect(result.retryAfter).toBe(60)
    }
  })

  it('returns error message on 5xx', async () => {
    server.use(
      http.post(`${BASE}/v1/events`, () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )
    const result = await postEvent({ event: 'page_viewed', user_id: 'user_1' })
    expect(result.ok).toBe(false)
    expect(result.status).toBe(500)
    if (!result.ok && 'message' in result) {
      expect(result.message).toContain('500')
    }
  })
})

describe('postEventsBatch', () => {
  it('returns ok with count on 202', async () => {
    const result = await postEventsBatch([
      { event: 'page_viewed', user_id: 'user_1' },
    ])
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.count).toBe(10)
    }
  })

  it('returns 429 with retryAfter on rate limit', async () => {
    server.use(
      http.post(`${BASE}/v1/events/batch`, () =>
        HttpResponse.json({}, { status: 429, headers: { 'Retry-After': '45' } }),
      ),
    )
    const result = await postEventsBatch([{ event: 'page_viewed', user_id: 'user_1' }])
    expect(result.ok).toBe(false)
    expect(result.status).toBe(429)
    if (!result.ok && result.status === 429) {
      expect(result.retryAfter).toBe(45)
    }
  })

  it('returns error on 5xx', async () => {
    server.use(
      http.post(`${BASE}/v1/events/batch`, () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )
    const result = await postEventsBatch([{ event: 'page_viewed', user_id: 'user_1' }])
    expect(result.ok).toBe(false)
    expect(result.status).toBe(500)
  })
})

describe('getStats', () => {
  it('returns stats shape', async () => {
    const stats = await getStats()
    expect(stats.total_events).toBe(42)
    expect(stats.today_count).toBe(7)
    expect(Array.isArray(stats.top_events)).toBe(true)
  })

  it('throws on non-ok response', async () => {
    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/stats`, () =>
        HttpResponse.json({}, { status: 503 }),
      ),
    )
    await expect(getStats()).rejects.toThrow('503')
  })
})

describe('listEvents', () => {
  it('returns full response with events and total', async () => {
    const data = await listEvents()
    expect(data.total).toBe(2)
    expect(data.events).toHaveLength(2)
    expect(data.events[0].event).toBe('page_viewed')
  })

  it('includes event filter in query string', async () => {
    let capturedUrl: string | undefined

    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/events`, ({ request }) => {
        capturedUrl = request.url
        return HttpResponse.json({ events: [], total: 0, limit: 20, offset: 0 })
      }),
    )

    await listEvents({ event: 'button_clicked' })
    expect(capturedUrl).toContain('event=button_clicked')
  })

  it('includes user_id filter in query string', async () => {
    let capturedUrl: string | undefined

    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/events`, ({ request }) => {
        capturedUrl = request.url
        return HttpResponse.json({ events: [], total: 0, limit: 20, offset: 0 })
      }),
    )

    await listEvents({ user_id: 'user_123' })
    expect(capturedUrl).toContain('user_id=user_123')
  })

  it('throws on non-ok response', async () => {
    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/events`, () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )
    await expect(listEvents()).rejects.toThrow('500')
  })
})

describe('listUserEvents', () => {
  it('returns events array for a user', async () => {
    const events = await listUserEvents('user_123')
    expect(events).toHaveLength(1)
    expect(events[0].user_id).toBe('user_123')
  })

  it('encodes userId in URL', async () => {
    let capturedUrl: string | undefined

    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/users/:uid/events`, ({ request }) => {
        capturedUrl = request.url
        return HttpResponse.json({ events: [], total: 0, limit: 20, offset: 0 })
      }),
    )

    await listUserEvents('user with spaces')
    expect(capturedUrl).toContain('user%20with%20spaces')
  })

  it('throws on non-ok response', async () => {
    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/users/:uid/events`, () =>
        HttpResponse.json({}, { status: 404 }),
      ),
    )
    await expect(listUserEvents('user_123')).rejects.toThrow('404')
  })
})

describe('getTopEvents', () => {
  it('returns events array', async () => {
    const events = await getTopEvents()
    expect(events).toHaveLength(2)
    expect(events[0].event).toBe('page_viewed')
    expect(events[0].count).toBe(20)
  })

  it('throws on non-ok response', async () => {
    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/events/top`, () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )
    await expect(getTopEvents()).rejects.toThrow('500')
  })
})
