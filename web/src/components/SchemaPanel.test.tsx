import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { SchemaPanel } from './SchemaPanel'
import * as api from '@/lib/api'

vi.mock('@/lib/api', () => ({
  listSchemas: vi.fn(),
  upsertSchema: vi.fn(),
  deleteSchema: vi.fn(),
  postEventWithProperties: vi.fn(),
}))

const mockListSchemas = vi.mocked(api.listSchemas)
const mockUpsertSchema = vi.mocked(api.upsertSchema)
const mockDeleteSchema = vi.mocked(api.deleteSchema)
const mockPostEventWithProperties = vi.mocked(api.postEventWithProperties)

function makeSchema(overrides: Partial<api.EventSchema> = {}): api.EventSchema {
  return {
    id: 'sc-1',
    project_id: 'proj-1',
    event_name: 'page_viewed',
    schema: { type: 'object', properties: { source: { type: 'string' } }, required: ['source'] },
    mode: 'warn',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('SchemaPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockListSchemas.mockResolvedValue([])
    mockUpsertSchema.mockResolvedValue(makeSchema())
    mockDeleteSchema.mockResolvedValue(undefined)
    mockPostEventWithProperties.mockResolvedValue({ ok: true, status: 202 })
  })

  it('shows empty state when no schemas exist', async () => {
    render(<SchemaPanel />)
    await waitFor(() => {
      expect(screen.getByText(/No schemas registered/)).toBeInTheDocument()
    })
  })

  it('registers schema and shows it in the list', async () => {
    render(<SchemaPanel />)
    await waitFor(() => mockListSchemas.mock.calls.length > 0)

    fireEvent.click(screen.getByRole('button', { name: /Register Schema/i }))

    await waitFor(() => {
      expect(screen.getByText('page_viewed')).toBeInTheDocument()
    })
    expect(mockUpsertSchema).toHaveBeenCalledOnce()
  })

  it('shows JSON parse error when schema textarea contains invalid JSON', async () => {
    render(<SchemaPanel />)
    await waitFor(() => mockListSchemas.mock.calls.length > 0)

    fireEvent.change(screen.getByLabelText(/JSON Schema/i), {
      target: { value: '{ invalid json' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Register Schema/i }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/Invalid JSON/i)
    })
    expect(mockUpsertSchema).not.toHaveBeenCalled()
  })

  it('removes schema from list on delete and shows empty state', async () => {
    mockListSchemas.mockResolvedValue([makeSchema()])
    render(<SchemaPanel />)

    await waitFor(() => screen.getByText(/Registered schemas/i))

    fireEvent.click(screen.getByRole('button', { name: /Delete schema for page_viewed/i }))

    await waitFor(() => {
      expect(screen.getByText(/No schemas registered/i)).toBeInTheDocument()
    })
    expect(mockDeleteSchema).toHaveBeenCalledWith('page_viewed')
  })

  it('shows 422 test result under the correct schema row only', async () => {
    mockListSchemas.mockResolvedValue([
      makeSchema({ event_name: 'page_viewed', mode: 'enforce' }),
      makeSchema({ id: 'sc-2', event_name: 'button_clicked', mode: 'enforce' }),
    ])
    mockPostEventWithProperties.mockResolvedValue({
      ok: false,
      status: 422,
      violations: ['properties/source: must be string'],
    })

    render(<SchemaPanel />)
    await waitFor(() => screen.getByText('page_viewed'))

    const testButtons = screen.getAllByText(/Demo: send a violating event/i)
    // Click the first schema's test button
    fireEvent.click(testButtons[0])

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent(/422/)
    })

    // Confirm the second row has no status element
    const statusEls = screen.getAllByRole('status')
    expect(statusEls).toHaveLength(1)
  })

  it('shows 202 test result in warn mode', async () => {
    mockListSchemas.mockResolvedValue([makeSchema({ mode: 'warn' })])
    mockPostEventWithProperties.mockResolvedValue({ ok: true, status: 202 })

    render(<SchemaPanel />)
    await waitFor(() => screen.getByText('page_viewed'))

    fireEvent.click(screen.getByText(/Demo: send a violating event/i))

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent(/202 accepted/i)
    })
  })

  it('calls onRequest callback after successful registration', async () => {
    const onRequest = vi.fn()
    render(<SchemaPanel onRequest={onRequest} />)
    await waitFor(() => mockListSchemas.mock.calls.length > 0)

    fireEvent.click(screen.getByRole('button', { name: /Register Schema/i }))

    await waitFor(() => expect(onRequest).toHaveBeenCalledWith(
      expect.objectContaining({ method: 'POST', status: 201 }),
    ))
  })

  it('shows mode badge on registered schema', async () => {
    mockListSchemas.mockResolvedValue([makeSchema({ mode: 'enforce' })])
    render(<SchemaPanel />)

    await waitFor(() => {
      expect(screen.getByText('enforce')).toBeInTheDocument()
    })
  })
})
