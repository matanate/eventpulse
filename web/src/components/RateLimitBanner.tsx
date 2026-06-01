import { useEffect, useState } from 'react'

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

  const progress = remaining / retryAfter

  return (
    <div
      role="status"
      aria-label={`Rate limited. Resets in ${remaining} seconds.`}
      className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm"
    >
      <div className="flex items-center justify-between">
        <span className="font-semibold text-amber-400">Rate limit reached</span>
        <span className="tabular-nums text-amber-300/70 text-xs">{remaining}s</span>
      </div>
      <div className="mt-2 h-1 overflow-hidden rounded-full bg-amber-500/20">
        <div
          className="h-full origin-left rounded-full bg-amber-400 transition-transform duration-1000 ease-linear"
          style={{ transform: `scaleX(${progress})` }}
        />
      </div>
    </div>
  )
}
