import { useState, useCallback } from 'react'
import { postEvent } from '../lib/api'
import { RateLimitBanner } from './RateLimitBanner'

const EVENT_TYPES = [
  'page_viewed',
  'user_signed_up',
  'checkout_completed',
  'button_clicked',
  'login_failed',
] as const

function randomUserId(): string {
  return `user_${Math.floor(Math.random() * 900) + 100}`
}

type FlashState = 'success' | 'error' | null

export function EventSender() {
  const [eventType, setEventType] = useState<string>(EVENT_TYPES[0])
  const [userId, setUserId] = useState(randomUserId)
  const [sending, setSending] = useState(false)
  const [sentCount, setSentCount] = useState(0)
  const [flash, setFlash] = useState<FlashState>(null)
  const [rateLimitSeconds, setRateLimitSeconds] = useState(0)
  const [errorMsg, setErrorMsg] = useState('')

  const handleSend = async () => {
    if (sending || rateLimitSeconds > 0) return
    setSending(true)
    setFlash(null)
    setErrorMsg('')

    try {
      const result = await postEvent({ event: eventType, user_id: userId })

      if (result.ok) {
        setSentCount((c) => c + 1)
        setFlash('success')
        setTimeout(() => setFlash(null), 1_500)
      } else if (result.status === 429 && 'retryAfter' in result) {
        setRateLimitSeconds(result.retryAfter)
      } else if ('message' in result) {
        setErrorMsg(result.message)
        setFlash('error')
        setTimeout(() => setFlash(null), 3_000)
      }
    } catch {
      setErrorMsg('Network error — check your connection')
      setFlash('error')
      setTimeout(() => setFlash(null), 3_000)
    } finally {
      setSending(false)
    }
  }

  const clearRateLimit = useCallback(() => setRateLimitSeconds(0), [])

  const btnClass = [
    'w-full rounded-lg px-4 py-2.5 text-sm font-semibold transition-colors duration-200',
    flash === 'success'
      ? 'bg-green-500 text-white'
      : flash === 'error'
        ? 'bg-red-500 text-white'
        : sending || rateLimitSeconds > 0
          ? 'bg-cyan-600/50 text-zinc-400 cursor-not-allowed'
          : 'bg-cyan-500 text-zinc-950 hover:bg-cyan-400',
  ].join(' ')

  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900 p-6 space-y-5">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-widest text-zinc-400">
          Send Event
        </h2>
        {sentCount > 0 && (
          <span className="rounded-full bg-cyan-500/10 px-2.5 py-0.5 text-xs font-medium text-cyan-400">
            {sentCount} sent
          </span>
        )}
      </div>

      <div className="space-y-3">
        <div className="space-y-1.5">
          <label className="text-xs text-zinc-500" htmlFor="event-type">
            Event type
          </label>
          <select
            id="event-type"
            value={eventType}
            onChange={(e) => setEventType(e.target.value)}
            className="w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-cyan-500 focus:outline-none"
          >
            {EVENT_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs text-zinc-500" htmlFor="user-id">
            User ID
          </label>
          <input
            id="user-id"
            type="text"
            value={userId}
            onChange={(e) => setUserId(e.target.value)}
            placeholder="user_123"
            className="w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-600 focus:border-cyan-500 focus:outline-none"
          />
        </div>
      </div>

      <button type="button" onClick={handleSend} className={btnClass}>
        {sending ? 'Sending…' : flash === 'success' ? '✓ Sent' : 'Send Event'}
      </button>

      {rateLimitSeconds > 0 && (
        <RateLimitBanner retryAfter={rateLimitSeconds} onExpire={clearRateLimit} />
      )}

      {flash === 'error' && errorMsg && (
        <p className="text-xs text-red-400">{errorMsg}</p>
      )}
    </div>
  )
}
