import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '../test/server'
import { EventSender } from './EventSender'
import { reportSuccess } from '../lib/connection'

afterEach(() => {
  reportSuccess() // Reset connection state
})

describe('EventSender — single mode', () => {
  it('renders the send button and mode tabs', () => {
    render(<EventSender />)
    expect(screen.getByRole('button', { name: /send event/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /single/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /batch/i })).toBeInTheDocument()
  })

  it('shows success flash after 202 response', async () => {
    render(<EventSender />)

    fireEvent.click(screen.getByRole('button', { name: /send event/i }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /✓ sent/i })).toBeInTheDocument(),
    )
  })

  it('increments sent count after successful send', async () => {
    render(<EventSender />)

    fireEvent.click(screen.getByRole('button', { name: /send event/i }))

    await waitFor(() => expect(screen.getByText('1 sent')).toBeInTheDocument())
  })

  it('shows rate limit banner on 429', async () => {
    server.use(
      http.post('http://test.local/v1/events', () =>
        HttpResponse.json({}, { status: 429, headers: { 'Retry-After': '60' } }),
      ),
    )

    render(<EventSender />)
    fireEvent.click(screen.getByRole('button', { name: /send event/i }))

    await waitFor(() =>
      expect(screen.getByText('Rate limit reached')).toBeInTheDocument(),
    )
  })

  it('shows error message on 5xx', async () => {
    server.use(
      http.post('http://test.local/v1/events', () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )

    render(<EventSender />)
    fireEvent.click(screen.getByRole('button', { name: /send event/i }))

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
  })
})

// Radix Tabs uses pointerDown internally — use userEvent for tab switching
describe('EventSender — batch mode', () => {
  beforeEach(async () => {
    render(<EventSender />)
    await userEvent.click(screen.getByRole('tab', { name: /batch/i }))
  })

  it('shows batch send button and size options', () => {
    expect(screen.getByRole('button', { name: /send 10 events/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '25' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '50' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '100' })).toBeInTheDocument()
  })

  it('changes batch size when size button is clicked', () => {
    fireEvent.click(screen.getByRole('button', { name: '50' }))
    expect(screen.getByRole('button', { name: /send 50 events/i })).toBeInTheDocument()
  })

  it('shows queued count after successful batch send', async () => {
    fireEvent.click(screen.getByRole('button', { name: /send 10 events/i }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /10 events queued/i })).toBeInTheDocument(),
    )
  })

  it('shows rate limit banner on 429 in batch mode', async () => {
    server.use(
      http.post('http://test.local/v1/events/batch', () =>
        HttpResponse.json({}, { status: 429, headers: { 'Retry-After': '30' } }),
      ),
    )

    fireEvent.click(screen.getByRole('button', { name: /send 10 events/i }))

    await waitFor(() =>
      expect(screen.getByText('Rate limit reached')).toBeInTheDocument(),
    )
  })

  it('shows error message on 5xx in batch mode', async () => {
    server.use(
      http.post('http://test.local/v1/events/batch', () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )

    fireEvent.click(screen.getByRole('button', { name: /send 10 events/i }))

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
  })
})
