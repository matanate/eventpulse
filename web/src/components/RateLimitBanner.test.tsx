import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { RateLimitBanner } from './RateLimitBanner'

beforeEach(() => { vi.useFakeTimers() })
afterEach(() => { vi.useRealTimers() })

describe('RateLimitBanner', () => {
  it('renders countdown text', () => {
    render(<RateLimitBanner retryAfter={30} onExpire={() => {}} />)
    expect(screen.getByText('30s')).toBeInTheDocument()
    expect(screen.getByText('Rate limit reached')).toBeInTheDocument()
  })

  it('renders progress bar', () => {
    render(<RateLimitBanner retryAfter={30} onExpire={() => {}} />)
    const el = document.querySelector('[style*="scaleX"]')
    expect(el).toBeTruthy()
  })

  it('counts down each second', async () => {
    render(<RateLimitBanner retryAfter={5} onExpire={() => {}} />)
    expect(screen.getByText('5s')).toBeInTheDocument()

    await act(async () => { vi.advanceTimersByTime(1000) })
    expect(screen.getByText('4s')).toBeInTheDocument()
  })

  it('calls onExpire when countdown reaches zero', async () => {
    const onExpire = vi.fn()
    render(<RateLimitBanner retryAfter={2} onExpire={onExpire} />)

    await act(async () => { vi.advanceTimersByTime(2000) })
    expect(onExpire).toHaveBeenCalledOnce()
  })

  it('renders nothing when retryAfter is 0', () => {
    const { container } = render(<RateLimitBanner retryAfter={0} onExpire={() => {}} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows fresh countdown when remounted with a new retryAfter (key-reset pattern)', () => {
    // Callers pass key={retryAfter} so the component remounts on new rate-limit windows
    const { unmount } = render(<RateLimitBanner retryAfter={10} onExpire={() => {}} />)
    expect(screen.getByText('10s')).toBeInTheDocument()

    unmount()
    render(<RateLimitBanner retryAfter={60} onExpire={() => {}} />)
    expect(screen.getByText('60s')).toBeInTheDocument()
  })
})
