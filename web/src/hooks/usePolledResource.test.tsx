import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { usePolledResource } from './usePolledResource'
import { reportSuccess } from '../lib/connection'

const INTERVAL = 80 // ms — short enough for fast tests

afterEach(() => {
  reportSuccess() // Reset connection state between tests
})

describe('usePolledResource', () => {
  it('starts with idle status and null data', () => {
    const fetcher = vi.fn(() => new Promise<never>(() => {})) // never resolves
    const { result } = renderHook(() => usePolledResource(fetcher, 60_000))

    expect(result.current.status).toBe('idle')
    expect(result.current.data).toBeNull()
    expect(result.current.lastUpdated).toBeNull()
  })

  it('updates to ok status and returns data after first fetch', async () => {
    const fetcher = vi.fn().mockResolvedValue({ value: 42 })
    const { result } = renderHook(() => usePolledResource(fetcher, 60_000))

    await waitFor(() => expect(result.current.status).toBe('ok'))

    expect(result.current.data).toEqual({ value: 42 })
    expect(fetcher).toHaveBeenCalledOnce()
  })

  it('polls again after interval elapses', async () => {
    const fetcher = vi.fn().mockResolvedValue({})
    renderHook(() => usePolledResource(fetcher, INTERVAL))

    await waitFor(
      () => expect(fetcher.mock.calls.length).toBeGreaterThanOrEqual(2),
      { timeout: 2000 },
    )
  })

  it('keeps last data when a subsequent fetch fails', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce({ value: 'first' })
      .mockRejectedValue(new Error('network error'))

    const { result } = renderHook(() => usePolledResource(fetcher, INTERVAL))

    await waitFor(() => expect(result.current.status).toBe('ok'))
    expect(result.current.data).toEqual({ value: 'first' })

    await waitFor(() => expect(result.current.status).toBe('error'))
    // Last known data is preserved on error
    expect(result.current.data).toEqual({ value: 'first' })
  })

  it('clears the interval on unmount', async () => {
    const fetcher = vi.fn().mockResolvedValue({})
    const { unmount } = renderHook(() => usePolledResource(fetcher, INTERVAL))

    await waitFor(() => expect(fetcher.mock.calls.length).toBeGreaterThanOrEqual(1))

    const countAtUnmount = fetcher.mock.calls.length
    unmount()

    await new Promise((r) => setTimeout(r, INTERVAL * 3))
    expect(fetcher.mock.calls.length).toBe(countAtUnmount)
  })

  it('refetch triggers an additional poll immediately', async () => {
    const fetcher = vi.fn().mockResolvedValue({})
    const { result } = renderHook(() => usePolledResource(fetcher, 60_000))

    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))

    act(() => { result.current.refetch() })

    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2))
  })

  it('always uses the latest fetcher on each poll', async () => {
    let currentValue = 1
    const fetcher = vi.fn().mockImplementation(
      () => Promise.resolve({ value: currentValue }),
    )

    const { result } = renderHook(() => usePolledResource(fetcher, INTERVAL))

    await waitFor(() => expect(result.current.data).toEqual({ value: 1 }))

    currentValue = 2

    await waitFor(() => expect(result.current.data).toEqual({ value: 2 }))
  })

  it('sets lastUpdated after a successful fetch', async () => {
    const fetcher = vi.fn().mockResolvedValue({})
    const { result } = renderHook(() => usePolledResource(fetcher, 60_000))

    await waitFor(() => expect(result.current.lastUpdated).toBeInstanceOf(Date))
  })
})
