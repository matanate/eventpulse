import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { reportFailure, reportSuccess } from '../lib/connection'

export type PolledStatus = 'idle' | 'ok' | 'error'

export interface PolledResource<T> {
  data: T | null
  status: PolledStatus
  lastUpdated: Date | null
  refetch: () => void
}

export function usePolledResource<T>(
  fetcher: () => Promise<T>,
  intervalMs: number,
): PolledResource<T> {
  const [data, setData] = useState<T | null>(null)
  const [status, setStatus] = useState<PolledStatus>('idle')
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const [triggerCount, setTriggerCount] = useState(0)
  const fetcherRef = useRef(fetcher)

  // Keep ref current after every render so the latest fetcher is always called
  useLayoutEffect(() => {
    fetcherRef.current = fetcher
  })

  useEffect(() => {
    let cancelled = false

    const poll = async () => {
      try {
        const result = await fetcherRef.current()
        if (!cancelled) {
          setData(result)
          setStatus('ok')
          setLastUpdated(new Date())
          reportSuccess()
        }
      } catch {
        if (!cancelled) {
          setStatus('error')
          reportFailure()
        }
      }
    }

    void poll()

    // intervalMs === 0 means one-shot: fetch once on mount, no recurring poll.
    if (intervalMs <= 0) {
      return () => { cancelled = true }
    }

    const id = setInterval(() => {
      void poll()
    }, intervalMs)

    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [intervalMs, triggerCount])

  const refetch = useCallback(() => setTriggerCount((n) => n + 1), [])

  return { data, status, lastUpdated, refetch }
}
