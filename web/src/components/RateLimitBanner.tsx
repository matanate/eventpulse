import { useEffect, useState } from 'react'

interface Props {
  retryAfter: number
  onExpire: () => void
}

export function RateLimitBanner({ retryAfter, onExpire }: Props) {
  const [remaining, setRemaining] = useState(retryAfter)

  useEffect(() => {
    setRemaining(retryAfter)
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

  return (
    <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm">
      <span className="font-semibold text-amber-400">Rate limit reached</span>
      <span className="ml-2 text-amber-300/70">— resets in {remaining}s</span>
    </div>
  )
}
