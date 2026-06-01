import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { usePolledResource, type PolledResource, type PolledStatus } from '../hooks/usePolledResource'
import type { StatsResult, EventsResponse } from '../lib/api'
import { StatsPanel } from './StatsPanel'

vi.mock('../hooks/usePolledResource')

const mockHook = vi.mocked(usePolledResource)

function makeStatsState(overrides: Partial<PolledResource<StatsResult>> = {}): PolledResource<StatsResult> {
  return {
    data: null,
    status: 'idle' as PolledStatus,
    lastUpdated: null,
    refetch: vi.fn(),
    ...overrides,
  }
}

function makeFeedState(overrides: Partial<PolledResource<EventsResponse>> = {}): PolledResource<EventsResponse> {
  return {
    data: null,
    status: 'idle' as PolledStatus,
    lastUpdated: null,
    refetch: vi.fn(),
    ...overrides,
  }
}

beforeEach(() => {
  mockHook.mockReset()
  // Default fallback for any call: empty stats state (null data)
  mockHook.mockReturnValue(makeStatsState() as unknown as PolledResource<never>)
})

describe('StatsPanel', () => {
  it('shows dashes when no data yet', () => {
    render(<StatsPanel />)
    expect(screen.getAllByText('—')).toHaveLength(4)
  })

  it('displays total_events and today_count when data is available', () => {
    mockHook
      .mockReturnValueOnce(makeStatsState({
        data: { total_events: 42, today_count: 7, top_events: [] },
        status: 'ok',
      }))
      .mockReturnValueOnce(makeFeedState())

    render(<StatsPanel />)

    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
  })

  it('shows formatted top event name', () => {
    mockHook
      .mockReturnValueOnce(makeStatsState({
        data: { total_events: 10, today_count: 2, top_events: [{ event: 'page_viewed', count: 5 }] },
        status: 'ok',
      }))
      .mockReturnValueOnce(makeFeedState())

    render(<StatsPanel />)

    expect(screen.getByText('Page Viewed')).toBeInTheDocument()
  })

  it('shows unique users from feed data', () => {
    mockHook
      .mockReturnValueOnce(makeStatsState())
      .mockReturnValueOnce(makeFeedState({
        data: {
          events: [
            { id: '1', event: 'page_viewed', user_id: 'user_a', timestamp: new Date().toISOString(), received_at: new Date().toISOString() },
            { id: '2', event: 'page_viewed', user_id: 'user_b', timestamp: new Date().toISOString(), received_at: new Date().toISOString() },
            { id: '3', event: 'page_viewed', user_id: 'user_a', timestamp: new Date().toISOString(), received_at: new Date().toISOString() },
          ],
          total: 3, limit: 200, offset: 0,
        },
        status: 'ok',
      }))

    render(<StatsPanel />)

    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('shows stale indicator when stats status is error', () => {
    mockHook
      .mockReturnValueOnce(makeStatsState({ status: 'error' }))
      .mockReturnValueOnce(makeFeedState())

    render(<StatsPanel />)

    expect(screen.getByRole('status', { name: /stale/i })).toBeInTheDocument()
  })

  it('does not show stale indicator when status is ok', () => {
    mockHook
      .mockReturnValueOnce(makeStatsState({
        data: { total_events: 10, today_count: 1, top_events: [] },
        status: 'ok',
      }))
      .mockReturnValueOnce(makeFeedState())

    render(<StatsPanel />)

    expect(screen.queryByRole('status', { name: /stale/i })).not.toBeInTheDocument()
  })
})
