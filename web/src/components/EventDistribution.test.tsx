import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { usePolledResource, type PolledResource, type PolledStatus } from '../hooks/usePolledResource'
import type { EventsResponse } from '../lib/api'
import { EventDistribution } from './EventDistribution'

vi.mock('../hooks/usePolledResource')
vi.mock('recharts', async () => {
  const actual = await vi.importActual<typeof import('recharts')>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
      <div style={{ width: 300, height: 300 }}>{children}</div>
    ),
  }
})

const mockHook = vi.mocked(usePolledResource)

function makeState(overrides: Partial<PolledResource<EventsResponse>> = {}): PolledResource<EventsResponse> {
  return {
    data: null,
    status: 'idle' as PolledStatus,
    lastUpdated: null,
    refetch: vi.fn(),
    ...overrides,
  }
}

const SAMPLE_EVENTS: EventsResponse = {
  events: [
    { id: '1', event: 'page_viewed',   user_id: 'u1', timestamp: new Date().toISOString(), received_at: new Date().toISOString() },
    { id: '2', event: 'page_viewed',   user_id: 'u2', timestamp: new Date().toISOString(), received_at: new Date().toISOString() },
    { id: '3', event: 'button_clicked', user_id: 'u1', timestamp: new Date().toISOString(), received_at: new Date().toISOString() },
  ],
  total: 3, limit: 200, offset: 0,
}

beforeEach(() => {
  mockHook.mockReturnValue(makeState())
})

describe('EventDistribution', () => {
  it('shows placeholder when no data', () => {
    render(<EventDistribution />)
    expect(screen.getByText('No data yet')).toBeInTheDocument()
  })

  it('renders formatted event names in legend', () => {
    mockHook.mockReturnValue(makeState({ data: SAMPLE_EVENTS, status: 'ok' }))
    render(<EventDistribution />)
    expect(screen.getByText('Page Viewed')).toBeInTheDocument()
    expect(screen.getByText('Button Clicked')).toBeInTheDocument()
  })

  it('shows percentage for each event type', () => {
    mockHook.mockReturnValue(makeState({ data: SAMPLE_EVENTS, status: 'ok' }))
    render(<EventDistribution />)
    expect(screen.getByText('67%')).toBeInTheDocument()
    expect(screen.getByText('33%')).toBeInTheDocument()
  })

  it('shows stale indicator on error status', () => {
    mockHook.mockReturnValue(makeState({ status: 'error' }))
    render(<EventDistribution />)
    expect(screen.getByRole('status', { name: /stale/i })).toBeInTheDocument()
  })

  it('renders the donut chart container', () => {
    mockHook.mockReturnValue(makeState({ data: SAMPLE_EVENTS, status: 'ok' }))
    render(<EventDistribution />)
    expect(screen.getByLabelText('Event distribution donut chart')).toBeInTheDocument()
  })
})
