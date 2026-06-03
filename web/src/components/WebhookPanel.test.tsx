import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { WebhookPanel } from './WebhookPanel'
import * as api from '@/lib/api'

vi.mock('@/lib/api', () => ({
  listWebhooks: vi.fn(),
  createWebhook: vi.fn(),
  deleteWebhook: vi.fn(),
}))

const mockListWebhooks = vi.mocked(api.listWebhooks)
const mockCreateWebhook = vi.mocked(api.createWebhook)
const mockDeleteWebhook = vi.mocked(api.deleteWebhook)

function makeWebhook(overrides: Partial<api.Webhook> = {}): api.Webhook {
  return {
    id: 'wh-1',
    url: 'https://webhook.site/test',
    active: true,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('WebhookPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockListWebhooks.mockResolvedValue([])
    mockCreateWebhook.mockResolvedValue(makeWebhook())
    mockDeleteWebhook.mockResolvedValue(undefined)
  })

  it('shows empty state when no webhooks exist', async () => {
    render(<WebhookPanel />)
    await waitFor(() => {
      expect(screen.getByText(/No webhooks yet/)).toBeInTheDocument()
    })
  })

  it('disables Register button when URL field is empty', async () => {
    render(<WebhookPanel />)
    await waitFor(() => mockListWebhooks.mock.calls.length > 0)

    const btn = screen.getByRole('button', { name: /Register Webhook/i })
    expect(btn).toBeDisabled()
  })

  it('enables Register button when URL is typed', async () => {
    render(<WebhookPanel />)
    fireEvent.change(screen.getByLabelText(/Endpoint URL/i), {
      target: { value: 'https://example.com/hook' },
    })
    expect(screen.getByRole('button', { name: /Register Webhook/i })).not.toBeDisabled()
  })

  it('adds webhook to list on successful create and clears URL field', async () => {
    render(<WebhookPanel />)

    fireEvent.change(screen.getByLabelText(/Endpoint URL/i), {
      target: { value: 'https://example.com/hook' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Register Webhook/i }))

    await waitFor(() => {
      expect(screen.getByText('https://webhook.site/test')).toBeInTheDocument()
    })
    expect(screen.getByLabelText(/Endpoint URL/i)).toHaveValue('')
  })

  it('shows error alert when create fails', async () => {
    mockCreateWebhook.mockRejectedValue(new Error('create webhook: HTTP 400 bad request'))
    render(<WebhookPanel />)

    fireEvent.change(screen.getByLabelText(/Endpoint URL/i), {
      target: { value: 'https://example.com/hook' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Register Webhook/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
  })

  it('removes webhook from list on delete', async () => {
    mockListWebhooks.mockResolvedValue([makeWebhook()])
    render(<WebhookPanel />)

    await waitFor(() => {
      expect(screen.getByText('https://webhook.site/test')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: /Delete webhook/i }))

    await waitFor(() => {
      expect(screen.queryByText('https://webhook.site/test')).not.toBeInTheDocument()
    })
  })

  it('shows error alert when delete fails', async () => {
    mockListWebhooks.mockResolvedValue([makeWebhook()])
    mockDeleteWebhook.mockRejectedValue(new Error('HTTP 500'))
    render(<WebhookPanel />)

    await waitFor(() => screen.getByRole('button', { name: /Delete webhook/i }))
    fireEvent.click(screen.getByRole('button', { name: /Delete webhook/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
  })

  it('shows webhook with filter_event badge when filter is set', async () => {
    mockListWebhooks.mockResolvedValue([makeWebhook({ filter_event: 'page_viewed' })])
    render(<WebhookPanel />)

    await waitFor(() => {
      expect(screen.getByText('page_viewed')).toBeInTheDocument()
    })
  })

  it('calls onRequest callback after successful create', async () => {
    const onRequest = vi.fn()
    render(<WebhookPanel onRequest={onRequest} />)

    fireEvent.change(screen.getByLabelText(/Endpoint URL/i), {
      target: { value: 'https://example.com/hook' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Register Webhook/i }))

    await waitFor(() => expect(onRequest).toHaveBeenCalledWith(
      expect.objectContaining({ method: 'POST', status: 201 }),
    ))
  })
})
