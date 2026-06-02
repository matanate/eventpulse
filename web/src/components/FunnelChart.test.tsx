import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { server } from '@/test/server'
import { http, HttpResponse } from 'msw'
import { FunnelChart } from './FunnelChart'

const BASE = 'http://test.local'
const PROJECT_ID = 'test-project-id'

const mockFunnelData = {
  steps: [
    { event: 'page_viewed',        entered: 120, converted: 72, dropped: 48, conversion_rate: 0.6 },
    { event: 'button_clicked',     entered: 72,  converted: 31, dropped: 41, conversion_rate: 0.431 },
    { event: 'checkout_completed', entered: 31,  converted: 0,  dropped: 0,  conversion_rate: 0 },
  ],
  window: 'P7D',
  overall_conversion_rate: 0.258,
}

describe('FunnelChart', () => {
  it('shows loading skeleton initially', () => {
    render(<FunnelChart />)
    expect(screen.getByLabelText('loading')).toBeInTheDocument()
  })

  it('renders step rows after data loads', async () => {
    render(<FunnelChart />)

    await waitFor(() => {
      expect(screen.getByText('Page Viewed')).toBeInTheDocument()
    })

    expect(screen.getByText('Button Clicked')).toBeInTheDocument()
    expect(screen.getByText('Checkout Completed')).toBeInTheDocument()
  })

  it('shows entered counts for each step', async () => {
    render(<FunnelChart />)

    await waitFor(() => {
      expect(screen.getByText('120')).toBeInTheDocument()
    })

    expect(screen.getByText('72')).toBeInTheDocument()
    expect(screen.getByText('31')).toBeInTheDocument()
  })

  it('shows overall conversion rate', async () => {
    render(<FunnelChart />)

    await waitFor(() => {
      expect(screen.getByText('25.8%')).toBeInTheDocument()
    })
  })

  it('shows error state when fetch fails', async () => {
    server.use(
      http.post(`${BASE}/v1/projects/${PROJECT_ID}/funnels`, () =>
        HttpResponse.error(),
      ),
    )

    render(<FunnelChart />)

    await waitFor(() => {
      expect(screen.getByText('Unable to load funnel data')).toBeInTheDocument()
    })
  })

  it('shows empty state when steps array is empty', async () => {
    server.use(
      http.post(`${BASE}/v1/projects/${PROJECT_ID}/funnels`, () =>
        HttpResponse.json({ steps: [], window: 'P7D', overall_conversion_rate: 0 }),
      ),
    )

    render(<FunnelChart />)

    await waitFor(() => {
      expect(screen.getByText('No data yet')).toBeInTheDocument()
    })
  })

  it('renders progress bars with correct aria labels', async () => {
    render(<FunnelChart />)

    await waitFor(() => {
      expect(screen.getByRole('progressbar', { name: /Page Viewed/ })).toBeInTheDocument()
    })

    const bars = screen.getAllByRole('progressbar')
    expect(bars).toHaveLength(mockFunnelData.steps.length)
  })
})
