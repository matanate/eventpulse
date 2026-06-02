const DEFAULT_BASE_DELAY_MS = 1_000

/** Retryable error wrapper — signals the caller should retry. */
export class RetryableError extends Error {
  readonly status: number | undefined
  constructor(message: string, status?: number) {
    super(message)
    this.name = 'RetryableError'
    this.status = status
  }
}

/** Non-retryable 429 error — carries the Retry-After header value in seconds. */
export class RateLimitError extends Error {
  readonly seconds: number
  constructor(seconds: number) {
    super(`rate limited — retry after ${seconds}s`)
    this.name = 'RateLimitError'
    this.seconds = seconds
  }
}

/**
 * Calls `fn` up to `maxAttempts` times, retrying on `RetryableError`
 * with exponential backoff: `baseDelayMs * 2^attempt`.
 *
 * Non-retryable errors (including 4xx) are re-thrown immediately.
 */
export async function withRetry<T>(
  fn: () => Promise<T>,
  maxAttempts: number,
  baseDelayMs = DEFAULT_BASE_DELAY_MS,
): Promise<T> {
  let attempt = 0
  while (true) {
    try {
      return await fn()
    } catch (err) {
      if (!(err instanceof RetryableError) || attempt >= maxAttempts - 1) {
        throw err
      }
      await sleep(baseDelayMs * 2 ** attempt)
      attempt++
    }
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
