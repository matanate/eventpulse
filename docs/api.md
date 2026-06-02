# EventPulse — API Reference

Base URL: `http://localhost:8080` (local dev)

All authenticated endpoints require `Authorization: Bearer <api-key>` where the key starts with `epk_`.

---

## Error Response Format

All errors return a JSON envelope:

```json
{
  "error": "human-readable message",
  "code": "MACHINE_READABLE_CODE",
  "details": [
    { "field": "event", "message": "event name is required" }
  ]
}
```

`details` is only present on `422 VALIDATION_FAILED` responses.

### Error Codes

| HTTP | Code | Cause |
|---|---|---|
| 400 | `INVALID_PAYLOAD` | Body is not valid JSON |
| 400 | `BATCH_EMPTY` | Batch request has zero events |
| 400 | `BATCH_TOO_LARGE` | Batch exceeds 100 events |
| 401 | `UNAUTHORIZED` | Missing or invalid API key |
| 403 | `FORBIDDEN` | API key does not own the requested project |
| 422 | `VALIDATION_FAILED` | One or more fields failed validation |
| 429 | `RATE_LIMITED` | Exceeded 100 requests/minute for this API key |
| 500 | `INTERNAL_ERROR` | Unexpected server error |

---

## Health Endpoints

### GET /healthz

Always returns 200. Use for liveness checks.

```bash
curl http://localhost:8080/healthz
```

```
200 OK
```

### GET /readyz

Returns 200 when both PostgreSQL and Redis are reachable. Returns 503 with a JSON body describing which dependency failed. Use for readiness checks.

```bash
curl http://localhost:8080/readyz
```

```json
200 OK
{ "status": "ok" }
```

```json
503 Service Unavailable
{ "status": "unavailable", "postgres": "ok", "redis": "dial tcp: connection refused" }
```

### GET /metrics

Prometheus text format metrics. No authentication required. Suitable for scraping by Prometheus.

```bash
curl http://localhost:8080/metrics
```

---

## Admin Endpoints

Admin endpoints require authentication but are **not** subject to per-key rate limiting.

### GET /v1/admin/queue/stats

Returns the current queue depth and dead-letter event count.

```bash
curl http://localhost:8080/v1/admin/queue/stats \
  -H "Authorization: Bearer epk_..."
```

```json
200 OK
{
  "pending_messages": 3,
  "dead_letter_count": 0
}
```

| Field | Type | Description |
|---|---|---|
| `pending_messages` | integer | Messages delivered to consumers but not yet ACKed |
| `dead_letter_count` | integer | Events that permanently failed after max retries |

---

## Ingestion Endpoints

All ingestion endpoints require authentication and are subject to rate limiting.

### POST /v1/events

Ingest a single event. Returns `202 Accepted` immediately — the event is enqueued for asynchronous storage.

**Request headers**

| Header | Required | Description |
|---|---|---|
| `Authorization` | Yes | `Bearer epk_...` API key |
| `Idempotency-Key` | No | UUID v4 — deduplicate retried requests (takes precedence over body field) |

**Request body**

| Field | Type | Required | Description |
|---|---|---|---|
| `event` | string | Yes | Event name (max 255 chars) |
| `user_id` | string | No | User identifier (max 255 chars) |
| `properties` | object | No | Arbitrary key-value metadata |
| `idempotency_key` | string | No | UUID v4 — deduplicate re-submitted events (header takes precedence) |
| `timestamp` | ISO 8601 string | No | Event time; defaults to server receive time |

**Idempotency**

Supply `Idempotency-Key: <UUID v4>` (or the `idempotency_key` body field) to enable safe retries. If two requests arrive with the same key for the same project within 24 hours, the second is accepted with `202` but produces no duplicate event. The dedup window is bounded by the project scope — keys are not globally unique across projects.

**Response**

```json
202 Accepted
{ "status": "queued" }
```

**Example**

```bash
curl -X POST http://localhost:8080/v1/events \
  -H "Authorization: Bearer epk_abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "event": "page_viewed",
    "user_id": "user-42",
    "properties": {
      "page": "/pricing",
      "referrer": "google"
    }
  }'
```

```json
{ "status": "queued" }
```

**With idempotency key** (safe to retry — duplicate is silently dropped):

```bash
curl -X POST http://localhost:8080/v1/events \
  -H "Authorization: Bearer epk_abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "event": "checkout_completed",
    "user_id": "user-42",
    "idempotency_key": "order-9981",
    "properties": { "amount": 49.99 }
  }'
```

---

### POST /v1/events/batch

Ingest 1–100 events in a single request. Validated atomically — if any event fails validation, the entire batch is rejected with details on which field(s) failed.

**Request body**

```json
{
  "events": [ <event object>, ... ]
}
```

Each event object has the same fields as the single-event endpoint.

**Response**

```json
202 Accepted
{ "count": 3, "status": "queued" }
```

**Example**

```bash
curl -X POST http://localhost:8080/v1/events/batch \
  -H "Authorization: Bearer epk_abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      { "event": "page_viewed", "user_id": "u1" },
      { "event": "button_clicked", "user_id": "u1", "properties": { "label": "Sign up" } },
      { "event": "form_submitted", "user_id": "u1" }
    ]
  }'
```

```json
{ "count": 3, "status": "queued" }
```

**Validation error example**

```json
422 Unprocessable Entity
{
  "error": "validation failed",
  "code": "VALIDATION_FAILED",
  "details": [
    { "field": "events[2].event", "message": "event name is required" }
  ]
}
```

---

## Analytics Endpoints

All analytics endpoints require authentication. The API key's project must match the `{projectID}` in the URL — mismatches return 403.

### GET /v1/projects/{projectID}/stats

Returns aggregate statistics for the project.

**Response**

```json
200 OK
{
  "total_events": 15420,
  "today_count": 342,
  "top_events": [
    { "event": "page_viewed", "count": 8200 },
    { "event": "button_clicked", "count": 4100 },
    { "event": "form_submitted", "count": 1800 },
    { "event": "checkout_started", "count": 920 },
    { "event": "purchase_completed", "count": 400 }
  ]
}
```

**Example**

```bash
curl http://localhost:8080/v1/projects/your-project-id/stats \
  -H "Authorization: Bearer epk_abc123..."
```

---

### GET /v1/projects/{projectID}/events

Paginated list of events. Supports optional filters.

**Query parameters**

| Param | Default | Description |
|---|---|---|
| `limit` | `50` | Results per page (max 100) |
| `offset` | `0` | Pagination offset |
| `event` | — | Filter by exact event name |
| `user_id` | — | Filter by user identifier |
| `from` | — | Start date (inclusive), format `YYYY-MM-DD` |
| `to` | — | End date (inclusive), format `YYYY-MM-DD` |

**Response**

```json
200 OK
{
  "events": [
    {
      "id": "018f5c7b-...",
      "project_id": "...",
      "event": "page_viewed",
      "user_id": "user-42",
      "properties": { "page": "/pricing" },
      "timestamp": "2026-05-31T10:22:00Z",
      "received_at": "2026-05-31T10:22:00.123Z"
    }
  ],
  "total": 15420,
  "limit": 50,
  "offset": 0
}
```

**Example — filter by event name and date range**

```bash
curl "http://localhost:8080/v1/projects/your-project-id/events?event=page_viewed&from=2026-05-01&to=2026-05-31&limit=20" \
  -H "Authorization: Bearer epk_abc123..."
```

---

### GET /v1/projects/{projectID}/events/top

Top N event names by count.

**Query parameters**

| Param | Default | Description |
|---|---|---|
| `n` | `10` | Number of results (max 50) |
| `from` | — | Start date (inclusive), `YYYY-MM-DD` |
| `to` | — | End date (inclusive), `YYYY-MM-DD` |

**Response**

```json
200 OK
{
  "events": [
    { "event": "page_viewed", "count": 8200 },
    { "event": "button_clicked", "count": 4100 }
  ],
  "n": 10
}
```

**Example**

```bash
curl "http://localhost:8080/v1/projects/your-project-id/events/top?n=5&from=2026-05-01" \
  -H "Authorization: Bearer epk_abc123..."
```

---

### GET /v1/projects/{projectID}/users/{userID}/events

All events for a specific user. Supports the same pagination and filter params as the events list endpoint (`limit`, `offset`, `event`, `from`, `to`).

**Response** — same shape as `GET /v1/projects/{projectID}/events`.

**Example**

```bash
curl "http://localhost:8080/v1/projects/your-project-id/users/user-42/events?limit=10" \
  -H "Authorization: Bearer epk_abc123..."
```

---

## Rate Limiting

Rate limits apply per API key, not per IP or project.

| Header | Description |
|---|---|
| `Retry-After` | Seconds until the window resets (only on 429 responses) |

**429 response**

```json
429 Too Many Requests
Retry-After: 42

{
  "error": "rate limit exceeded",
  "code": "RATE_LIMITED"
}
```

The `Retry-After` value is precise: it is the ceiling of milliseconds until the oldest entry in the sliding window expires, converted to seconds.

---

## Quick Start

```bash
# 1. Start infrastructure
make infra-up

# 2. Apply migrations (first time)
docker exec -i deploy-postgres-1 psql -U eventpulse -d eventpulse \
  < migrations/000001_init.up.sql

# 3. Seed an API key
make seed
# → prints API Key: epk_...

# 4. Start the API
export DATABASE_URL=postgres://eventpulse:eventpulse@localhost:5433/eventpulse?sslmode=disable
export REDIS_URL=redis://localhost:6379
make run-api

# 5. Send an event
curl -X POST http://localhost:8080/v1/events \
  -H "Authorization: Bearer epk_<your-key>" \
  -H "Content-Type: application/json" \
  -d '{"event": "hello_world", "user_id": "test-user"}'
```
