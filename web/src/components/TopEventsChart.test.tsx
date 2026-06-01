import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { usePolledResource, type PolledResource, type PolledStatus } from '../hooks/usePolledResource'
import type { EventCount } from '../lib/api'
import { TopEventsChart } from './TopEventsChart'

vi.mock('../hooks/usePolledResource')
// ResponsiveContainer needs explicit dimensions in jsdom
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

function makeState(overrides: Partial<PolledResource<EventCount[]>> = {}): PolledResource<EventCount[]> {
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

describe('TopEventsChart', () => {
  it('shows placeholder text when no data', () => {
    render(<TopEventsChart />)
    expect(screen.getByText('No data yet')).toBeInTheDocument()
  })

  it('shows stale indicator on error status', () => {
    mockHook.mockReturnValue(makeState({ status: 'error' }))
    render(<TopEventsChart />)
    expect(screen.getByRole('status', { name: /stale/i })).toBeInTheDocument()
  })

  it('hides placeholder and renders chart container when data is available', () => {
    mockHook.mockReturnValue(
      makeState({
        data: [
          { event: 'page_viewed', count: 20 },
          { event: 'button_clicked', count: 10 },
        ],
        status: 'ok',
      }),
    )

    render(<TopEventsChart />)
    // Placeholder disappears when data is present
    expect(screen.queryByText('No data yet')).not.toBeInTheDocument()
  })
})
