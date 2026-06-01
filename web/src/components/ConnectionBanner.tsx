import { useEffect, useState } from 'react'
import { getConnectionStatus, subscribeToConnection } from '@/lib/connection'
import { Alert, AlertDescription } from '@/components/ui/alert'

export function ConnectionBanner() {
  const [online, setOnline] = useState(getConnectionStatus())

  useEffect(() => {
    return subscribeToConnection(() => setOnline(getConnectionStatus()))
  }, [])

  if (online) return null

  return (
    <Alert variant="destructive" className="rounded-none border-x-0 border-t-0 py-2">
      <AlertDescription className="text-center text-xs">
        API unreachable — retrying automatically…
      </AlertDescription>
    </Alert>
  )
}
