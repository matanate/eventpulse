import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useEventSource } from './useEventSource'
import type { EventRow } from '../lib/api'

// Minimal EventSource mock
class MockEventSource {
  static instances: MockEventSource[] = []
  url: string
  onopen: (() => void) | null = null
  onmessage: ((evt: MessageEvent) => void) | null = null
  onerror: (() => void) | null = null
  readyState = 0

  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }

  close() {
    this.readyState = 2
  }

  // Test helpers
  simulateOpen() {
    this.readyState = 1
    this.onopen?.()
  }
  simulateMessage(data: string) {
    this.onmessage?.(new MessageEvent('message', { data }))
  }
  simulateError() {
    this.onerror?.()
  }
}

const makeEvent = (id: string): EventRow => ({
  id,
  event: 'test_event',
  user_id: 'u1',
  timestamp: new Date().toISOString(),
  received_at: new Date().toISOString(),
})

beforeEach(() => {
  MockEventSource.instances = []
  vi.stubGlobal('EventSource', MockEventSource)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useEventSource', () => {
  it('starts with connecting status', () => {
    const { result } = renderHook(() => useEventSource('http://test/stream'))
    expect(result.current.status).toBe('connecting')
    expect(result.current.events).toHaveLength(0)
  })

  it('transitions to open on connect', () => {
    const { result } = renderHook(() => useEventSource('http://test/stream'))

    act(() => {
      MockEventSource.instances[0].simulateOpen()
    })

    expect(result.current.status).toBe('open')
  })

  it('prepends new events to the buffer', () => {
    const { result } = renderHook(() => useEventSource('http://test/stream'))

    act(() => {
      MockEventSource.instances[0].simulateOpen()
      MockEventSource.instances[0].simulateMessage(JSON.stringify(makeEvent('e1')))
      MockEventSource.instances[0].simulateMessage(JSON.stringify(makeEvent('e2')))
    })

    expect(result.current.events[0].id).toBe('e2')
    expect(result.current.events[1].id).toBe('e1')
  })

  it('caps the ring buffer at 100 events', () => {
    const { result } = renderHook(() => useEventSource('http://test/stream'))

    act(() => {
      MockEventSource.instances[0].simulateOpen()
      for (let i = 0; i < 110; i++) {
        MockEventSource.instances[0].simulateMessage(JSON.stringify(makeEvent(`e${i}`)))
      }
    })

    expect(result.current.events).toHaveLength(100)
  })

  it('seeds initial events from prop', () => {
    const seed = [makeEvent('seed-1'), makeEvent('seed-2')]
    const { result } = renderHook(() => useEventSource('http://test/stream', seed))

    expect(result.current.events).toHaveLength(2)
    expect(result.current.events[0].id).toBe('seed-1')
  })

  it('deduplicates when SSE event matches seed', () => {
    const seed = [makeEvent('e1')]
    const { result } = renderHook(() => useEventSource('http://test/stream', seed))

    act(() => {
      MockEventSource.instances[0].simulateOpen()
      MockEventSource.instances[0].simulateMessage(JSON.stringify(makeEvent('e1')))
    })

    expect(result.current.events.filter((e) => e.id === 'e1')).toHaveLength(1)
  })

  it('sets error status on connection failure', () => {
    const { result } = renderHook(() => useEventSource('http://test/stream'))

    act(() => {
      MockEventSource.instances[0].simulateError()
    })

    expect(result.current.status).toBe('error')
  })

  it('does not connect when url is null', () => {
    renderHook(() => useEventSource(null))
    expect(MockEventSource.instances).toHaveLength(0)
  })

  it('ignores malformed JSON messages', () => {
    const { result } = renderHook(() => useEventSource('http://test/stream'))

    act(() => {
      MockEventSource.instances[0].simulateOpen()
      MockEventSource.instances[0].simulateMessage('not-json')
    })

    expect(result.current.events).toHaveLength(0)
    expect(result.current.status).toBe('open')
  })

  it('closes EventSource on unmount', () => {
    const { unmount } = renderHook(() => useEventSource('http://test/stream'))
    const es = MockEventSource.instances[0]

    unmount()

    expect(es.readyState).toBe(2)
  })
})
