import type { EventPulseConfig, TrackPayload, FlushResult } from './types'
import { AutoBatcher } from './batcher'

const DEFAULT_FLUSH_INTERVAL_MS = 1_000
const DEFAULT_MAX_QUEUE_SIZE = 100
const DEFAULT_MAX_RETRIES = 3

export class EventPulseClient {
  private readonly batcher: AutoBatcher

  constructor(config: EventPulseConfig) {
    this.batcher = new AutoBatcher({
      endpoint: config.endpoint,
      apiKey: config.apiKey,
      flushIntervalMs: config.flushIntervalMs ?? DEFAULT_FLUSH_INTERVAL_MS,
      maxQueueSize: config.maxQueueSize ?? DEFAULT_MAX_QUEUE_SIZE,
      maxRetries: config.maxRetries ?? DEFAULT_MAX_RETRIES,
    })
  }

  /**
   * Queue an event for auto-batched delivery.
   * A UUID v4 idempotency key is generated automatically.
   */
  track(event: string, userId: string, properties?: Record<string, unknown>): void {
    this.batcher.enqueue({
      event,
      userId,
      properties,
      idempotencyKey: crypto.randomUUID(),
    })
  }

  /**
   * Record user identity traits as an "identify" event.
   */
  identify(userId: string, traits?: Record<string, unknown>): void {
    this.track('identify', userId, traits)
  }

  /**
   * Immediately send an explicit list of events, bypassing the auto-batch queue.
   * Each event gets its own idempotency key.
   */
  async trackBatch(events: TrackPayload[]): Promise<FlushResult> {
    const queued = events.map((e) => ({
      ...e,
      idempotencyKey: crypto.randomUUID(),
    }))
    return this.batcher.sendBatch(queued)
  }

  /**
   * Force-flush all queued events immediately.
   * Returns when the HTTP call completes (or fails after retries).
   */
  flush(): Promise<FlushResult> {
    return this.batcher.flush()
  }

  /**
   * Stop the periodic flush timer. Call on component unmount or page unload.
   */
  destroy(): void {
    this.batcher.destroy()
  }
}
