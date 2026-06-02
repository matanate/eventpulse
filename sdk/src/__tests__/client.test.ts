import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { EventPulseClient } from '../client'

const CONFIG = {
  endpoint: 'http://api.test',
  apiKey: 'epk_test',
  flushIntervalMs: 60_000,
  maxRetries: 1,
}

function mockFetch(status: number) {
  return vi.fn().mockResolvedValue(new Response(JSON.stringify({ count: 1, status: 'queued' }), { status }))
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('fetch', mockFetch(202))
  vi.stubGlobal('crypto', {
    randomUUID: vi.fn().mockReturnValue('test-uuid-1234'),
  })
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('EventPulseClient', () => {
  it('track + flush sends event to batch endpoint', async () => {
    const client = new EventPulseClient(CONFIG)
    client.track('page_viewed', 'u1', { page: '/home' })
    const result = await client.flush()
    client.destroy()

    expect(result).toEqual({ sent: 1, failed: 0 })

    const body = JSON.parse(vi.mocked(fetch).mock.calls[0]![1]!.body as string) as {
      events: Array<{ event: string; user_id: string; idempotency_key: string }>
    }
    expect(body.events[0]).toMatchObject({
      event: 'page_viewed',
      user_id: 'u1',
      idempotency_key: 'test-uuid-1234',
    })
  })

  it('identify queues an "identify" event', async () => {
    const client = new EventPulseClient(CONFIG)
    client.identify('u1', { name: 'Alice' })
    await client.flush()
    client.destroy()

    const body = JSON.parse(vi.mocked(fetch).mock.calls[0]![1]!.body as string) as {
      events: Array<{ event: string }>
    }
    expect(body.events[0]?.event).toBe('identify')
  })

  it('trackBatch sends events directly without queuing', async () => {
    const client = new EventPulseClient(CONFIG)
    const result = await client.trackBatch([
      { event: 'signup', userId: 'u1' },
      { event: 'login', userId: 'u2' },
    ])
    client.destroy()

    expect(result).toEqual({ sent: 2, failed: 0 })
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1)
  })

  it('each track call gets a unique idempotency key', async () => {
    vi.mocked(crypto.randomUUID)
      .mockReturnValueOnce('uuid-a' as `${string}-${string}-${string}-${string}-${string}`)
      .mockReturnValueOnce('uuid-b' as `${string}-${string}-${string}-${string}-${string}`)

    const client = new EventPulseClient(CONFIG)
    client.track('e1', 'u1')
    client.track('e2', 'u2')
    await client.flush()
    client.destroy()

    const body = JSON.parse(vi.mocked(fetch).mock.calls[0]![1]!.body as string) as {
      events: Array<{ idempotency_key: string }>
    }
    expect(body.events[0]?.idempotency_key).toBe('uuid-a')
    expect(body.events[1]?.idempotency_key).toBe('uuid-b')
  })

  it('sets Authorization header correctly', async () => {
    const client = new EventPulseClient({ ...CONFIG, apiKey: 'epk_secret' })
    client.track('e', 'u')
    await client.flush()
    client.destroy()

    const headers = vi.mocked(fetch).mock.calls[0]![1]!.headers as Record<string, string>
    expect(headers['Authorization']).toBe('Bearer epk_secret')
  })

  it('destroy stops the auto-flush timer', async () => {
    const client = new EventPulseClient({ ...CONFIG, flushIntervalMs: 100 })
    client.track('e', 'u')
    client.destroy()

    await vi.advanceTimersByTimeAsync(200)
    expect(vi.mocked(fetch)).not.toHaveBeenCalled()
  })
})
