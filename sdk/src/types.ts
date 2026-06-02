export interface EventPulseConfig {
  endpoint: string
  apiKey: string
  /** Milliseconds between automatic queue flushes. Default: 1000 */
  flushIntervalMs?: number
  /** Flush immediately when queue reaches this size. Default: 100 */
  maxQueueSize?: number
  /** Maximum retry attempts on 5xx / network errors. Default: 3 */
  maxRetries?: number
}

export interface TrackPayload {
  event: string
  userId: string
  properties?: Record<string, unknown>
}

export interface QueuedEvent extends TrackPayload {
  idempotencyKey: string
}

export interface FlushResult {
  sent: number
  failed: number
  /** Present when the server responded 429 — seconds from the Retry-After header. */
  rateLimitSeconds?: number
}
