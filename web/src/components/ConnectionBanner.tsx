import { useEffect, useState } from 'react'
import { getConnectionStatus, subscribeToConnection } from '../lib/connection'

export function ConnectionBanner() {
  const [online, setOnline] = useState(getConnectionStatus())

  useEffect(() => {
    return subscribeToConnection(() => setOnline(getConnectionStatus()))
  }, [])

  if (online) return null

  return (
    <div
      role="alert"
      className="border-b border-red-500/30 bg-red-500/10 px-6 py-2 text-center"
    >
      <span className="text-xs font-medium text-red-400">
        API unreachable — retrying automatically…
      </span>
    </div>
  )
}
