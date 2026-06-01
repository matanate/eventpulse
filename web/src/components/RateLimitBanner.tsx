import { useEffect, useState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Progress } from '@/components/ui/progress'

interface Props {
  retryAfter: number
  onExpire: () => void
}

export function RateLimitBanner({ retryAfter, onExpire }: Props) {
  const [remaining, setRemaining] = useState(retryAfter)

  useEffect(() => {
    if (retryAfter <= 0) return

    const id = setInterval(() => {
      setRemaining((r) => {
        if (r <= 1) {
          clearInterval(id)
          onExpire()
          return 0
        }
        return r - 1
      })
    }, 1000)

    return () => clearInterval(id)
  }, [retryAfter, onExpire])

  if (remaining <= 0) return null

  const progress = Math.round((remaining / retryAfter) * 100)

  return (
    <Alert className="border-amber-500/30 bg-amber-500/10 text-amber-400">
      <AlertDescription className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="font-semibold text-amber-400">Rate limit reached</span>
          <span className="tabular-nums text-amber-300/70 text-xs">{remaining}s</span>
        </div>
        <Progress
          value={progress}
          className="h-1 bg-amber-500/20 [&>div]:bg-amber-400"
        />
      </AlertDescription>
    </Alert>
  )
}
