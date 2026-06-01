import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { usePolledResource, type PolledResource, type PolledStatus } from '../hooks/usePolledResource'
import type { EventsResponse } from '../lib/api'
import { EventFeed } from './EventFeed'

vi.mock('../hooks/usePolledResource')

const mockHook = vi.mocked(usePolledResource)

const EVT_WITH_PROPS: EventsResponse = {
  events: [
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
  ],
  total: 2,
  limit: 20,
  offset: 0,
}

function makeState(overrides: Partial<PolledResource<EventsResponse>> = {}): PolledResource<EventsResponse> {
  return {
    data: null,
    status: 'idle' as PolledStatus,
    lastUpdated: null,
    refetch: vi.fn(),
    ...overrides,
  }
}

beforeEach(() => {
  mockHook.mockReturnValue(makeState())
})

describe('EventFeed', () => {
  it('shows empty state when no data', () => {
    render(<EventFeed />)
    expect(screen.getByText(/No events yet/)).toBeInTheDocument()
  })

  it('renders events from feed data', () => {
    mockHook.mockReturnValue(makeState({ data: EVT_WITH_PROPS, status: 'ok' }))

    render(<EventFeed />)

    // user_id buttons are unique to the event feed (not in the filter dropdown)
    expect(screen.getByText('user_123')).toBeInTheDocument()
    expect(screen.getByText('user_456')).toBeInTheDocument()
  })

  it('shows expand button for events that have properties', () => {
    mockHook.mockReturnValue(makeState({ data: EVT_WITH_PROPS, status: 'ok' }))

    render(<EventFeed />)

    // Only evt-1 has non-empty properties
    const expandBtns = screen.getAllByLabelText('Toggle properties')
    expect(expandBtns).toHaveLength(1)
  })

  it('expands properties inline when toggle button is clicked', () => {
    mockHook.mockReturnValue(makeState({ data: EVT_WITH_PROPS, status: 'ok' }))

    render(<EventFeed />)

    const expandBtn = screen.getByLabelText('Toggle properties')
    fireEvent.click(expandBtn)

    expect(screen.getByText('page:')).toBeInTheDocument()
    expect(screen.getByText('/home')).toBeInTheDocument()
  })

  it('collapses properties when toggle button is clicked again', () => {
    mockHook.mockReturnValue(makeState({ data: EVT_WITH_PROPS, status: 'ok' }))

    render(<EventFeed />)

    const expandBtn = screen.getByLabelText('Toggle properties')
    fireEvent.click(expandBtn) // expand
    fireEvent.click(expandBtn) // collapse

    expect(screen.queryByText('page:')).not.toBeInTheDocument()
  })

  it('clicking a user_id sets the userId filter input', () => {
    mockHook.mockReturnValue(makeState({ data: EVT_WITH_PROPS, status: 'ok' }))

    render(<EventFeed />)

    const userBtn = screen.getByText('user_123')
    fireEvent.click(userBtn)

    const input = screen.getByLabelText('Filter by user ID') as HTMLInputElement
    expect(input.value).toBe('user_123')
  })

  it('shows empty state with filter message when filters are active and no data', () => {
    mockHook.mockReturnValue(makeState({ data: { events: [], total: 0, limit: 20, offset: 0 }, status: 'ok' }))

    render(<EventFeed />)

    // Trigger filter
    const input = screen.getByLabelText('Filter by user ID')
    fireEvent.change(input, { target: { value: 'nobody' } })

    expect(screen.getByText(/No events match/)).toBeInTheDocument()
  })

  it('shows stale indicator when status is error', () => {
    mockHook.mockReturnValue(makeState({ status: 'error' }))

    render(<EventFeed />)

    expect(screen.getByRole('status', { name: /stale/i })).toBeInTheDocument()
  })

  it('shows total count when events are present', () => {
    mockHook.mockReturnValue(makeState({ data: EVT_WITH_PROPS, status: 'ok' }))

    render(<EventFeed />)

    expect(screen.getByText('2/2')).toBeInTheDocument()
  })
})
