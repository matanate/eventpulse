import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { EventPulseClient } from '@eventpulse/client'
import type { FlushResult } from '@eventpulse/client'
import { EventSender } from './EventSender'
import { reportSuccess } from '../lib/connection'

// Mock the SDK so component tests don't make real HTTP requests.
// SDK internals (batching, retry, idempotency) are tested in sdk/src/__tests__/.
vi.mock('@eventpulse/client')

const mockTrack = vi.fn()
const mockTrackBatch = vi.fn<[], Promise<FlushResult>>()
const mockFlush = vi.fn<[], Promise<FlushResult>>()
const mockDestroy = vi.fn()

beforeEach(() => {
  // Vitest 4.x requires a class expression for constructor mocks.
  // Class fields reference the shared mock fns so per-test overrides work.
  vi.mocked(EventPulseClient).mockImplementation(class {
    track = mockTrack
    identify = vi.fn()
    trackBatch = mockTrackBatch
    flush = mockFlush
    destroy = mockDestroy
  } as unknown as typeof EventPulseClient)
  mockFlush.mockResolvedValue({ sent: 1, failed: 0 })
  mockTrackBatch.mockResolvedValue({ sent: 10, failed: 0 })
  mockTrack.mockReturnValue(undefined)
})

afterEach(() => {
  vi.clearAllMocks()
  reportSuccess()
})

// ──────────────────────────────────────────────────────────────────────────────
// Single mode
// ──────────────────────────────────────────────────────────────────────────────

describe('EventSender — single mode', () => {
  it('renders the send button and mode tabs', () => {
    render(<EventSender />)
    expect(screen.getByRole('button', { name: /send event/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /single/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /batch/i })).toBeInTheDocument()
  })

  it('renders the auth failure demo link', () => {
    render(<EventSender />)
    expect(screen.getByText(/demo: try invalid api key/i)).toBeInTheDocument()
  })

  it('renders the idempotency duplicate demo link', () => {
    render(<EventSender />)
    expect(screen.getByText(/demo: send duplicate/i)).toBeInTheDocument()
  })

  it('shows success flash after SDK flush resolves sent=1', async () => {
    render(<EventSender />)
    fireEvent.click(screen.getByRole('button', { name: /send event/i }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /✓ sent/i })).toBeInTheDocument(),
    )
    expect(mockTrack).toHaveBeenCalledTimes(1)
    expect(mockFlush).toHaveBeenCalledTimes(1)
  })

  it('increments sent count after successful send', async () => {
    render(<EventSender />)
    fireEvent.click(screen.getByRole('button', { name: /send event/i }))

    await waitFor(() => expect(screen.getByText('1 sent')).toBeInTheDocument())
  })

  it('shows rate limit banner when flush returns rateLimitSeconds', async () => {
    mockFlush.mockResolvedValue({ sent: 0, failed: 1, rateLimitSeconds: 60 })

    render(<EventSender />)
    fireEvent.click(screen.getByRole('button', { name: /send event/i }))

    await waitFor(() =>
      expect(screen.getByText('Rate limit reached')).toBeInTheDocument(),
    )
  })

  it('shows error alert when flush returns sent=0 with no rateLimitSeconds', async () => {
    mockFlush.mockResolvedValue({ sent: 0, failed: 1 })

    render(<EventSender />)
    fireEvent.click(screen.getByRole('button', { name: /send event/i }))

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
  })

  it('calls onRequest with 202 on success', async () => {
    const onRequest = vi.fn()
    render(<EventSender onRequest={onRequest} />)

    fireEvent.click(screen.getByRole('button', { name: /send event/i }))
    await waitFor(() => expect(onRequest).toHaveBeenCalled())

    expect(onRequest).toHaveBeenCalledWith(
      expect.objectContaining({ status: 202 }),
    )
  })
})

// ──────────────────────────────────────────────────────────────────────────────
// Demo buttons — use real api.ts (MSW handles the HTTP calls)
// ──────────────────────────────────────────────────────────────────────────────

describe('EventSender — demo buttons', () => {
  it('shows idempotency proof message after duplicate demo', async () => {
    const onRequest = vi.fn()
    render(<EventSender onRequest={onRequest} />)

    fireEvent.click(screen.getByText(/demo: send duplicate/i))

    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(/2× accepted · 1 unique event/i),
    )
    expect(onRequest).toHaveBeenCalledTimes(2)
  })
})

// ──────────────────────────────────────────────────────────────────────────────
// Batch mode
// ──────────────────────────────────────────────────────────────────────────────

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
    expect(mockTrackBatch).toHaveBeenCalledWith(
      expect.arrayContaining([expect.objectContaining({ event: expect.any(String) })]),
    )
  })

  it('shows rate limit banner on 429 in batch mode', async () => {
    mockTrackBatch.mockResolvedValue({ sent: 0, failed: 10, rateLimitSeconds: 30 })

    fireEvent.click(screen.getByRole('button', { name: /send 10 events/i }))

    await waitFor(() =>
      expect(screen.getByText('Rate limit reached')).toBeInTheDocument(),
    )
  })

  it('shows error alert on failure in batch mode', async () => {
    mockTrackBatch.mockResolvedValue({ sent: 0, failed: 10 })

    fireEvent.click(screen.getByRole('button', { name: /send 10 events/i }))

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
  })
})
