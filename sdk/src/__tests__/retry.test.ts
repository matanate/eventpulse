import { describe, it, expect, vi, beforeEach } from 'vitest'
import { withRetry, RetryableError } from '../retry'

// Speed up tests — no real sleeping
vi.mock('../retry', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../retry')>()
  return mod
})

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('withRetry', () => {
  it('returns result on first success', async () => {
    const fn = vi.fn().mockResolvedValue('ok')
    const result = await withRetry(fn, 3, 0)
    expect(result).toBe('ok')
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('retries on RetryableError and succeeds', async () => {
    const fn = vi
      .fn()
      .mockRejectedValueOnce(new RetryableError('server error'))
      .mockResolvedValue('ok')

    const promise = withRetry(fn, 3, 0)
    await vi.runAllTimersAsync()
    const result = await promise

    expect(result).toBe('ok')
    expect(fn).toHaveBeenCalledTimes(2)
  })

  it('exhausts retries and throws after maxAttempts', async () => {
    const fn = vi.fn().mockRejectedValue(new RetryableError('always fails'))

    const promise = withRetry(fn, 3, 0)
    promise.catch(() => {}) // suppress unhandled-rejection warning before timers fire
    await vi.runAllTimersAsync()

    await expect(promise).rejects.toThrow('always fails')
    expect(fn).toHaveBeenCalledTimes(3)
  })

  it('does not retry non-retryable errors', async () => {
    const fn = vi.fn().mockRejectedValue(new Error('client 400'))

    const promise = withRetry(fn, 3, 0)
    promise.catch(() => {}) // suppress unhandled-rejection warning
    await vi.runAllTimersAsync()

    await expect(promise).rejects.toThrow('client 400')
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('applies exponential backoff between retries', async () => {
    const delays: number[] = []
    const origSetTimeout = globalThis.setTimeout
    vi.spyOn(globalThis, 'setTimeout').mockImplementation((fn, ms, ...args) => {
      delays.push(ms as number)
      return origSetTimeout(fn, 0, ...args)
    })

    const fn = vi
      .fn()
      .mockRejectedValueOnce(new RetryableError('e1'))
      .mockRejectedValueOnce(new RetryableError('e2'))
      .mockResolvedValue('ok')

    const promise = withRetry(fn, 3, 100)
    await vi.runAllTimersAsync()
    await promise

    // First retry: 100 * 2^0 = 100, second retry: 100 * 2^1 = 200
    expect(delays).toContain(100)
    expect(delays).toContain(200)
  })
})
