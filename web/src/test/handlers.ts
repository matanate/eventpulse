import { http, HttpResponse } from 'msw'

const BASE = 'http://test.local'
const PROJECT_ID = 'test-project-id'

export const handlers = [
  // POST /v1/events → 202
  http.post(`${BASE}/v1/events`, () =>
    HttpResponse.json({ status: 'queued' }, { status: 202 }),
  ),

  // POST /v1/events (rate limited variant — used in overrideHandlers)
  // POST /v1/events/batch → 202
  http.post(`${BASE}/v1/events/batch`, () =>
    HttpResponse.json({ count: 10, status: 'queued' }, { status: 202 }),
  ),

  // GET /v1/projects/:id/stats
  http.get(`${BASE}/v1/projects/${PROJECT_ID}/stats`, () =>
    HttpResponse.json({
      total_events: 42,
      today_count: 7,
      top_events: [{ event: 'page_viewed', count: 20 }],
    }),
  ),

  // GET /v1/projects/:id/events
  http.get(`${BASE}/v1/projects/${PROJECT_ID}/events`, () =>
    HttpResponse.json({
      events: [
        {
          id: 'evt-1',
          event: 'page_viewed',
          user_id: 'user_123',
          properties: { page: '/home' },
          timestamp: new Date().toISOString(),
          received_at: new Date().toISOString(),
        },
        {
          id: 'evt-2',
          event: 'button_clicked',
          user_id: 'user_456',
          properties: {},
          timestamp: new Date().toISOString(),
          received_at: new Date().toISOString(),
        },
      ],
      total: 2,
      limit: 20,
      offset: 0,
    }),
  ),

  // GET /v1/projects/:id/events/top
  http.get(`${BASE}/v1/projects/${PROJECT_ID}/events/top`, () =>
    HttpResponse.json({
      events: [
        { event: 'page_viewed', count: 20 },
        { event: 'button_clicked', count: 10 },
      ],
      n: 5,
    }),
  ),

  // GET /v1/projects/:id/users/:uid/events
  http.get(`${BASE}/v1/projects/${PROJECT_ID}/users/:uid/events`, () =>
    HttpResponse.json({
      events: [
        {
          id: 'evt-u1',
          event: 'page_viewed',
          user_id: 'user_123',
          properties: {},
          timestamp: new Date().toISOString(),
          received_at: new Date().toISOString(),
        },
      ],
      total: 1,
      limit: 20,
      offset: 0,
    }),
  ),
]
