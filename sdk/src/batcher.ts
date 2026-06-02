import type { QueuedEvent, FlushResult } from './types'
import { withRetry, RetryableError, RateLimitError } from './retry'

interface BatcherConfig {
  endpoint: string
  apiKey: string
  flushIntervalMs: number
  maxQueueSize: number
  maxRetries: number
}

/** Serialized event shape expected by POST /v1/events/batch */
interface BatchEventPayload {
  event: string
  user_id: string
  properties?: Record<string, unknown>
  idempotency_key: string
}

export class AutoBatcher {
  private readonly config: BatcherConfig
  private queue: QueuedEvent[] = []
  private timer: ReturnType<typeof setInterval> | null = null
  private flushInFlight: Promise<FlushResult> | null = null

  constructor(config: BatcherConfig) {
    this.config = config
    this.timer = setInterval(() => {
      void this.flush()
    }, config.flushIntervalMs)
  }

  enqueue(event: QueuedEvent): void {
    this.queue.push(event)
    if (this.queue.length >= this.config.maxQueueSize) {
      void this.flush()
    }
  }

  /**
   * Drains the current queue in a single batch HTTP call.
   * Concurrent callers share the same in-flight promise.
   */
  flush(): Promise<FlushResult> {
    if (this.flushInFlight !== null) return this.flushInFlight

    const batch = this.queue.splice(0)
    if (batch.length === 0) return Promise.resolve({ sent: 0, failed: 0 })

    this.flushInFlight = this.send(batch).finally(() => {
      this.flushInFlight = null
    })

    return this.flushInFlight
  }

  /** Send a pre-formed batch directly, bypassing the queue. */
  async sendBatch(events: QueuedEvent[]): Promise<FlushResult> {
    if (events.length === 0) return { sent: 0, failed: 0 }
    return this.send(events)
  }

  destroy(): void {
    if (this.timer !== null) {
      clearInterval(this.timer)
      this.timer = null
    }
  }

  private async send(batch: QueuedEvent[]): Promise<FlushResult> {
    const payload: BatchEventPayload[] = batch.map((e) => ({
      event: e.event,
      user_id: e.userId,
      ...(e.properties !== undefined ? { properties: e.properties } : {}),
      idempotency_key: e.idempotencyKey,
    }))

    try {
      await withRetry(
        () => this.postBatch(payload),
        this.config.maxRetries,
      )
      return { sent: batch.length, failed: 0 }
    } catch (err) {
      if (err instanceof RateLimitError) {
        return { sent: 0, failed: batch.length, rateLimitSeconds: err.seconds }
      }
      return { sent: 0, failed: batch.length }
    }
  }

  private async postBatch(events: BatchEventPayload[]): Promise<void> {
    let res: Response
    try {
      res = await fetch(`${this.config.endpoint}/v1/events/batch`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${this.config.apiKey}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ events }),
      })
    } catch (err) {
      throw new RetryableError(`network error: ${String(err)}`)
    }

    if (res.ok) return

    if (res.status === 429) {
      const parsed = parseInt(res.headers.get('Retry-After') ?? '', 10)
      const seconds = Number.isFinite(parsed) && parsed > 0 ? parsed : 60
      throw new RateLimitError(seconds)
    }

    if (res.status >= 500) {
      throw new RetryableError(`server error ${res.status}`, res.status)
    }

    // Other 4xx — not retryable
    throw new Error(`client error ${res.status}`)
  }
}
