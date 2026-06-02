import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { server } from '@/test/server'
import { http, HttpResponse } from 'msw'
import { RetentionGrid } from './RetentionGrid'

const BASE = 'http://test.local'
const PROJECT_ID = 'test-project-id'

const mockRetentionData = {
  period: 'day',
  cohorts: 4,
  rows: [
    {
      cohort_date: '2026-05-31',
      cohort_size: 1,
      buckets: [{ offset: 0, count: 1, rate: 1.0 }],
    },
    {
      cohort_date: '2026-05-30',
      cohort_size: 2,
      buckets: [
        { offset: 0, count: 2, rate: 1.0 },
        { offset: 1, count: 1, rate: 0.5 },
      ],
    },
    {
      cohort_date: '2026-05-29',
      cohort_size: 4,
      buckets: [
        { offset: 0, count: 4, rate: 1.0 },
        { offset: 1, count: 3, rate: 0.75 },
        { offset: 2, count: 2, rate: 0.5 },
      ],
    },
    {
      cohort_date: '2026-05-28',
      cohort_size: 5,
      buckets: [
        { offset: 0, count: 5, rate: 1.0 },
        { offset: 1, count: 4, rate: 0.8 },
        { offset: 2, count: 3, rate: 0.6 },
        { offset: 3, count: 2, rate: 0.4 },
      ],
    },
  ],
}

describe('RetentionGrid', () => {
  it('shows loading skeleton initially', () => {
    render(<RetentionGrid />)
    expect(screen.getByLabelText('loading')).toBeInTheDocument()
  })

  it('renders column headers D+0 through D+3 after data loads', async () => {
    render(<RetentionGrid />)

    await waitFor(() => {
      expect(screen.getByText('D+0')).toBeInTheDocument()
    })

    expect(screen.getByText('D+1')).toBeInTheDocument()
    expect(screen.getByText('D+2')).toBeInTheDocument()
    expect(screen.getByText('D+3')).toBeInTheDocument()
  })

  it('renders cohort date labels', async () => {
    render(<RetentionGrid />)

    await waitFor(() => {
      // MSW returns cohort_date '2026-05-28' → "May 28"
      expect(screen.getByText('May 28')).toBeInTheDocument()
    })

    expect(screen.getByText('May 29')).toBeInTheDocument()
    expect(screen.getByText('May 30')).toBeInTheDocument()
    expect(screen.getByText('May 31')).toBeInTheDocument()
  })

  it('renders cohort size labels', async () => {
    render(<RetentionGrid />)

    await waitFor(() => {
      expect(screen.getByText('n=5')).toBeInTheDocument()
    })

    expect(screen.getByText('n=4')).toBeInTheDocument()
    expect(screen.getByText('n=2')).toBeInTheDocument()
    expect(screen.getByText('n=1')).toBeInTheDocument()
  })

  it('renders D+0 cells with 100.0% for all cohorts', async () => {
    render(<RetentionGrid />)

    await waitFor(() => {
      const hundredCells = screen.getAllByText('100.0%')
      expect(hundredCells.length).toBe(mockRetentionData.rows.length)
    })
  })

  it('renders percentage text in non-D+0 cells', async () => {
    render(<RetentionGrid />)

    await waitFor(() => {
      expect(screen.getByText('75.0%')).toBeInTheDocument()
    })

    // 50.0% appears in two cells (May 30 D+1 and May 29 D+2)
    expect(screen.getAllByText('50.0%').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('80.0%')).toBeInTheDocument()
    expect(screen.getByText('60.0%')).toBeInTheDocument()
    expect(screen.getByText('40.0%')).toBeInTheDocument()
  })

  it('shows error state when fetch fails', async () => {
    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/retention`, () => HttpResponse.error()),
    )

    render(<RetentionGrid />)

    await waitFor(() => {
      expect(screen.getByText('Unable to load retention data')).toBeInTheDocument()
    })
  })

  it('shows empty state when rows array is empty', async () => {
    server.use(
      http.get(`${BASE}/v1/projects/${PROJECT_ID}/retention`, () =>
        HttpResponse.json({ period: 'day', cohorts: 4, rows: [] }),
      ),
    )

    render(<RetentionGrid />)

    await waitFor(() => {
      expect(screen.getByText('No data yet')).toBeInTheDocument()
    })
  })
})
