import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { useEventSource, type EventSourceState, type SSEStatus } from '../hooks/useEventSource'
import { usePolledResource } from '../hooks/usePolledResource'
import type { EventRow, EventsResponse } from '../lib/api'
import { EventFeed } from './EventFeed'

vi.mock('../hooks/useEventSource')
vi.mock('../hooks/usePolledResource')

const mockSSEHook = vi.mocked(useEventSource)
const mockPolledHook = vi.mocked(usePolledResource)

const EVENTS: EventRow[] = [
  {
    id: 'evt-1',
    event: 'page_viewed',
    user_id: 'user_123',
    properties: { page: '/home' },
    timestamp: new Date().toISOString(),
    received_at: new Date().toISOString(),
  },
  {
    id: 'evt-2',
    event: 'button_clicked',
    user_id: 'user_456',
    properties: {},
    timestamp: new Date().toISOString(),
    received_at: new Date().toISOString(),
  },
]

function makeSSEState(overrides: Partial<EventSourceState> = {}): EventSourceState {
  return { events: [], status: 'open' as SSEStatus, reconnectCount: 0, ...overrides }
}

// Silence the polling hooks used for seed + fallback
function silencePolled() {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mockPolledHook.mockReturnValue({ data: null, status: 'idle', lastUpdated: null, refetch: vi.fn() } as any)
}

beforeEach(() => {
  silencePolled()
  mockSSEHook.mockReturnValue(makeSSEState())
})

describe('EventFeed', () => {
  it('shows empty state when no events', () => {
    render(<EventFeed />)
    expect(screen.getByText(/No events yet/)).toBeInTheDocument()
  })

  it('renders events from SSE buffer', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ events: EVENTS }))

    render(<EventFeed />)

    expect(screen.getByText('user_123')).toBeInTheDocument()
    expect(screen.getByText('user_456')).toBeInTheDocument()
  })

  it('shows expand button only for events with non-empty properties', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ events: EVENTS }))

    render(<EventFeed />)

    const expandBtns = screen.getAllByLabelText('Toggle properties')
    expect(expandBtns).toHaveLength(1)
  })

  it('expands properties inline when toggle button is clicked', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ events: EVENTS }))

    render(<EventFeed />)

    const expandBtn = screen.getByLabelText('Toggle properties')
    fireEvent.click(expandBtn)

    expect(screen.getByText('page:')).toBeInTheDocument()
    expect(screen.getByText('/home')).toBeInTheDocument()
  })

  it('collapses properties when toggle button is clicked again', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ events: EVENTS }))

    render(<EventFeed />)

    const expandBtn = screen.getByLabelText('Toggle properties')
    fireEvent.click(expandBtn)
    fireEvent.click(expandBtn)

    expect(screen.queryByText('page:')).not.toBeInTheDocument()
  })

  it('clicking a user_id sets the userId filter input', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ events: EVENTS }))

    render(<EventFeed />)

    fireEvent.click(screen.getByText('user_123'))

    const input = screen.getByLabelText('Filter by user ID') as HTMLInputElement
    expect(input.value).toBe('user_123')
  })

  it('shows empty state with filter message when filters are active and no events match', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ events: EVENTS }))

    render(<EventFeed />)

    fireEvent.change(screen.getByLabelText('Filter by user ID'), { target: { value: 'nobody' } })

    expect(screen.getByText(/No events match/)).toBeInTheDocument()
  })

  it('shows stale indicator when SSE status is error', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ status: 'error' }))
    // Silence fallback poll
    mockPolledHook.mockReturnValue({ data: null, status: 'idle', lastUpdated: null, refetch: vi.fn() } as any) // eslint-disable-line @typescript-eslint/no-explicit-any

    render(<EventFeed />)

    expect(screen.getByRole('status', { name: /stale/i })).toBeInTheDocument()
  })

  it('shows event count badge when events are present', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ events: EVENTS }))

    render(<EventFeed />)

    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('shows connecting indicator when SSE is connecting', () => {
    mockSSEHook.mockReturnValue(makeSSEState({ status: 'connecting' }))

    render(<EventFeed />)

    expect(screen.getByLabelText('connecting')).toBeInTheDocument()
  })

  it('uses fallback polling data when SSE is in error state', () => {
    const fallbackEvents: EventRow[] = [
      {
        id: 'fallback-1',
        event: 'fallback_event',
        timestamp: new Date().toISOString(),
        received_at: new Date().toISOString(),
      },
    ]
    const fallbackResponse: EventsResponse = { events: fallbackEvents, total: 1, limit: 20, offset: 0 }

    mockSSEHook.mockReturnValue(makeSSEState({ status: 'error', events: [] }))
    mockPolledHook
      .mockReturnValueOnce({ data: null, status: 'idle', lastUpdated: null, refetch: vi.fn() } as any) // eslint-disable-line @typescript-eslint/no-explicit-any
      .mockReturnValue({ data: fallbackResponse, status: 'ok', lastUpdated: new Date(), refetch: vi.fn() } as any) // eslint-disable-line @typescript-eslint/no-explicit-any

    render(<EventFeed />)

    expect(screen.getByText('Fallback Event')).toBeInTheDocument()
  })
})
