import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { usePolledResource, type PolledResource, type PolledStatus } from '../hooks/usePolledResource'
import type { StatsResult } from '../lib/api'
import { StatsPanel } from './StatsPanel'

vi.mock('../hooks/usePolledResource')

const mockHook = vi.mocked(usePolledResource)

function makeState(overrides: Partial<PolledResource<StatsResult>> = {}): PolledResource<StatsResult> {
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

describe('StatsPanel', () => {
  it('shows dashes when no data yet', () => {
    render(<StatsPanel />)
    expect(screen.getAllByText('—')).toHaveLength(2)
  })

  it('displays total_events and today_count when data is available', () => {
    mockHook.mockReturnValue(
      makeState({
        data: { total_events: 42, today_count: 7, top_events: [] },
        status: 'ok',
      }),
    )

    render(<StatsPanel />)

    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
  })

  it('shows stale indicator when status is error', () => {
    mockHook.mockReturnValue(makeState({ status: 'error' }))

    render(<StatsPanel />)

    expect(screen.getByRole('status', { name: /stale/i })).toBeInTheDocument()
  })

  it('shows stale indicator AND preserves last data on error', () => {
    mockHook.mockReturnValue(
      makeState({
        data: { total_events: 99, today_count: 3, top_events: [] },
        status: 'error',
      }),
    )

    render(<StatsPanel />)

    expect(screen.getByText('99')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByRole('status', { name: /stale/i })).toBeInTheDocument()
  })

  it('does not show stale indicator when status is ok', () => {
    mockHook.mockReturnValue(
      makeState({
        data: { total_events: 10, today_count: 1, top_events: [] },
        status: 'ok',
      }),
    )

    render(<StatsPanel />)

    expect(screen.queryByRole('status', { name: /stale/i })).not.toBeInTheDocument()
  })
})
