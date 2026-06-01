import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  getConnectionStatus,
  subscribeToConnection,
  reportSuccess,
  reportFailure,
} from './connection'

// Reset module state between tests by re-importing
beforeEach(async () => {
  vi.resetModules()
})

describe('connection store', () => {
  it('starts online', async () => {
    const { getConnectionStatus: getStatus } = await import('./connection')
    expect(getStatus()).toBe(true)
  })

  it('reportFailure sets status to false and notifies listeners', async () => {
    const { getConnectionStatus: getStatus, reportFailure: fail, subscribeToConnection: sub } =
      await import('./connection')

    const listener = vi.fn()
    sub(listener)

    fail()
    expect(getStatus()).toBe(false)
    expect(listener).toHaveBeenCalledOnce()
  })

  it('reportSuccess sets status to true and notifies listeners', async () => {
    const { reportFailure: fail, reportSuccess: succeed, getConnectionStatus: getStatus, subscribeToConnection: sub } =
      await import('./connection')

    const listener = vi.fn()
    sub(listener)

    fail() // go offline first
    succeed()
    expect(getStatus()).toBe(true)
    expect(listener).toHaveBeenCalledTimes(2)
  })

  it('does not notify if status did not change', async () => {
    const { reportSuccess: succeed, subscribeToConnection: sub } =
      await import('./connection')

    const listener = vi.fn()
    sub(listener)

    // Already online, calling reportSuccess again should not notify
    succeed()
    expect(listener).not.toHaveBeenCalled()
  })

  it('unsubscribe removes listener', async () => {
    const { reportFailure: fail, subscribeToConnection: sub } =
      await import('./connection')

    const listener = vi.fn()
    const unsub = sub(listener)
    unsub()

    fail()
    expect(listener).not.toHaveBeenCalled()
  })
})

// Smoke tests using the named exports (no module reset needed, just verifying the API shape)
describe('connection module exports', () => {
  it('exports getConnectionStatus as a function', () => {
    expect(typeof getConnectionStatus).toBe('function')
  })
  it('exports subscribeToConnection as a function', () => {
    expect(typeof subscribeToConnection).toBe('function')
  })
  it('exports reportSuccess as a function', () => {
    expect(typeof reportSuccess).toBe('function')
  })
  it('exports reportFailure as a function', () => {
    expect(typeof reportFailure).toBe('function')
  })
})
