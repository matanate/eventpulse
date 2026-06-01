type Listener = () => void

let isOnline = true
const listeners = new Set<Listener>()

export function getConnectionStatus(): boolean {
  return isOnline
}

export function subscribeToConnection(fn: Listener): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

function notify(status: boolean): void {
  if (isOnline === status) return
  isOnline = status
  for (const fn of listeners) fn()
}

export function reportSuccess(): void {
  notify(true)
}

export function reportFailure(): void {
  notify(false)
}
