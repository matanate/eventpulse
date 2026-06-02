import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { AutoBatcher } from '../batcher'
import type { QueuedEvent } from '../types'

const ENDPOINT = 'http://api.test'
const API_KEY = 'epk_test'

function makeConfig(overrides?: Partial<Parameters<typeof AutoBatcher>[0]>) {
  return {
    endpoint: ENDPOINT,
    apiKey: API_KEY,
    flushIntervalMs: 1_000,
    maxQueueSize: 100,
    maxRetries: 1,
    ...overrides,
  }
}

function makeEvent(id: string): QueuedEvent {
  return { event: 'test', userId: 'u1', idempotencyKey: id }
}

function mockFetch(status: number) {
  return vi.fn().mockResolvedValue(new Response(null, { status }))
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('fetch', mockFetch(202))
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('AutoBatcher', () => {
  it('flushes nothing when queue is empty', async () => {
    const batcher = new AutoBatcher(makeConfig())
    const result = await batcher.flush()
    batcher.destroy()
    expect(result).toEqual({ sent: 0, failed: 0 })
    expect(vi.mocked(fetch)).not.toHaveBeenCalled()
  })

  it('sends queued events on flush', async () => {
    const batcher = new AutoBatcher(makeConfig())
    batcher.enqueue(makeEvent('k1'))
    batcher.enqueue(makeEvent('k2'))

    const result = await batcher.flush()
    batcher.destroy()

    expect(result).toEqual({ sent: 2, failed: 0 })
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1)

    const body = JSON.parse(vi.mocked(fetch).mock.calls[0]![1]!.body as string) as {
      events: unknown[]
    }
    expect(body.events).toHaveLength(2)
  })

  it('includes idempotency_key in each event payload', async () => {
    const batcher = new AutoBatcher(makeConfig())
    batcher.enqueue(makeEvent('uuid-1'))
    await batcher.flush()
    batcher.destroy()

    const body = JSON.parse(vi.mocked(fetch).mock.calls[0]![1]!.body as string) as {
      events: Array<{ idempotency_key: string }>
    }
    expect(body.events[0]?.idempotency_key).toBe('uuid-1')
  })

  it('auto-flushes when queue reaches maxQueueSize', async () => {
    const batcher = new AutoBatcher(makeConfig({ maxQueueSize: 3, maxRetries: 1 }))

    batcher.enqueue(makeEvent('k1'))
    batcher.enqueue(makeEvent('k2'))
    batcher.enqueue(makeEvent('k3')) // triggers auto-flush via void this.flush()

    // Join the in-flight promise started by the auto-trigger (guarded by flushInFlight)
    await batcher.flush()
    batcher.destroy()

    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1)
  })

  it('auto-flushes on interval', async () => {
    const batcher = new AutoBatcher(makeConfig({ flushIntervalMs: 500 }))
    batcher.enqueue(makeEvent('k1'))

    await vi.advanceTimersByTimeAsync(500)
    batcher.destroy()

    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1)
  })

  it('returns failed count on server error', async () => {
    vi.stubGlobal('fetch', mockFetch(500))

    const batcher = new AutoBatcher(makeConfig({ maxRetries: 1 }))
    batcher.enqueue(makeEvent('k1'))
    const result = await batcher.flush()
    batcher.destroy()

    expect(result).toEqual({ sent: 0, failed: 1 })
  })

  it('returns rateLimitSeconds on 429', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(null, { status: 429, headers: { 'Retry-After': '42' } }),
      ),
    )

    const batcher = new AutoBatcher(makeConfig({ maxRetries: 1 }))
    batcher.enqueue(makeEvent('k1'))
    const result = await batcher.flush()
    batcher.destroy()

    expect(result).toEqual({ sent: 0, failed: 1, rateLimitSeconds: 42 })
  })

  it('does not overlap concurrent flushes', async () => {
    const batcher = new AutoBatcher(makeConfig())
    batcher.enqueue(makeEvent('k1'))

    const p1 = batcher.flush()
    const p2 = batcher.flush()

    const [r1, r2] = await Promise.all([p1, p2])
    batcher.destroy()

    // Both promises resolve but only one HTTP call was made
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1)
    expect(r1).toEqual({ sent: 1, failed: 0 })
    expect(r2).toEqual({ sent: 1, failed: 0 })
  })

  it('sendBatch sends provided events immediately', async () => {
    const batcher = new AutoBatcher(makeConfig())
    const result = await batcher.sendBatch([makeEvent('k1'), makeEvent('k2')])
    batcher.destroy()

    expect(result).toEqual({ sent: 2, failed: 0 })
  })

  it('destroy stops the interval timer', async () => {
    const batcher = new AutoBatcher(makeConfig({ flushIntervalMs: 200 }))
    batcher.enqueue(makeEvent('k1'))
    batcher.destroy()

    await vi.advanceTimersByTimeAsync(400)

    expect(vi.mocked(fetch)).not.toHaveBeenCalled()
  })
})
