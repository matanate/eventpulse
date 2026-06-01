import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { ConnectionBanner } from './ConnectionBanner'
import { reportFailure, reportSuccess } from '../lib/connection'

beforeEach(() => {
  reportSuccess() // Ensure we start online before each test
})

describe('ConnectionBanner', () => {
  it('renders nothing when online', () => {
    const { container } = render(<ConnectionBanner />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the banner when offline', () => {
    render(<ConnectionBanner />)

    act(() => { reportFailure() })

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText(/API unreachable/)).toBeInTheDocument()
  })

  it('hides the banner when back online', () => {
    render(<ConnectionBanner />)

    act(() => { reportFailure() })
    expect(screen.getByRole('alert')).toBeInTheDocument()

    act(() => { reportSuccess() })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
