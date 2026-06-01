import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { usePolledResource, type PolledResource, type PolledStatus } from '../hooks/usePolledResource'
import type { EventsResponse } from '../lib/api'
import { ActivityTimeline } from './ActivityTimeline'

vi.mock('../hooks/usePolledResource')
vi.mock('recharts', async () => {
  const actual = await vi.importActual<typeof import('recharts')>('recharts')
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
      <div style={{ width: 400, height: 200 }}>{children}</div>
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

beforeEach(() => {
  mockHook.mockReturnValue(makeState())
})

describe('ActivityTimeline', () => {
  it('renders the card header', () => {
    render(<ActivityTimeline />)
    expect(screen.getByText('Ingestion Rate')).toBeInTheDocument()
  })

  it('renders the chart aria label', () => {
    render(<ActivityTimeline />)
    expect(screen.getByLabelText('Event ingestion rate over time')).toBeInTheDocument()
  })

  it('shows stale indicator on error status', () => {
    mockHook.mockReturnValue(makeState({ status: 'error' }))
    render(<ActivityTimeline />)
    expect(screen.getByRole('status', { name: /stale/i })).toBeInTheDocument()
  })

  it('renders without error when events are provided', () => {
    const now = new Date()
    mockHook.mockReturnValue(makeState({
      data: {
        events: [
          { id: '1', event: 'page_viewed', user_id: 'u1', timestamp: now.toISOString(), received_at: now.toISOString() },
          { id: '2', event: 'button_clicked', user_id: 'u2', timestamp: now.toISOString(), received_at: now.toISOString() },
        ],
        total: 2, limit: 200, offset: 0,
      },
      status: 'ok',
    }))

    render(<ActivityTimeline />)
    expect(screen.getByText('Ingestion Rate')).toBeInTheDocument()
  })
})
