import { http, HttpResponse } from 'msw'

const BASE = 'http://test.local'
const PROJECT_ID = 'test-project-id'

export const handlers = [
  // POST /v1/events → check Authorization header: bad key gets 401, good key gets 202
  http.post(`${BASE}/v1/events`, ({ request }) => {
    const auth = request.headers.get('Authorization') ?? ''
    if (auth === 'Bearer invalid_key_demo') {
      return HttpResponse.json({ code: 'UNAUTHORIZED', message: 'invalid api key' }, { status: 401 })
    }
    return HttpResponse.json({ status: 'queued' }, { status: 202 })
  }),

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

  // POST /v1/projects/:id/funnels
  http.post(`${BASE}/v1/projects/${PROJECT_ID}/funnels`, () =>
    HttpResponse.json({
      steps: [
        { event: 'page_viewed',       entered: 120, converted: 72, dropped: 48, conversion_rate: 0.6 },
        { event: 'button_clicked',    entered: 72,  converted: 31, dropped: 41, conversion_rate: 0.431 },
        { event: 'checkout_completed',entered: 31,  converted: 0,  dropped: 0,  conversion_rate: 0 },
      ],
      window: 'P7D',
      overall_conversion_rate: 0.258,
    }),
  ),
]
