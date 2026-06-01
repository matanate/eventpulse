import { useState, useCallback } from 'react'
import { postEvent, postEventsBatch, type PostEventPayload } from '../lib/api'
import { RateLimitBanner } from './RateLimitBanner'

const EVENT_TYPES = [
  'page_viewed',
  'user_signed_up',
  'checkout_completed',
  'button_clicked',
  'login_failed',
] as const

type EventType = (typeof EVENT_TYPES)[number]
type Mode = 'single' | 'batch'
type FlashState = 'success' | 'error' | null

const BATCH_COUNTS = [10, 25, 50, 100] as const

function randomUserId(): string {
  return `user_${Math.floor(Math.random() * 900) + 100}`
}

function generateBatch(count: number): PostEventPayload[] {
  return Array.from({ length: count }, () => ({
    event: EVENT_TYPES[Math.floor(Math.random() * EVENT_TYPES.length)] as string,
    user_id: randomUserId(),
    properties: { source: 'batch_demo' },
  }))
}

export function EventSender() {
  const [mode, setMode] = useState<Mode>('single')

  // Single mode state
  const [eventType, setEventType] = useState<string>(EVENT_TYPES[0])
  const [userId, setUserId] = useState(randomUserId)
  const [sentCount, setSentCount] = useState(0)
  const [flash, setFlash] = useState<FlashState>(null)
  const [errorMsg, setErrorMsg] = useState('')

  // Batch mode state
  const [batchCount, setBatchCount] = useState<number>(10)
  const [batchFlash, setBatchFlash] = useState<FlashState>(null)
  const [batchResult, setBatchResult] = useState<number | null>(null)
  const [batchErrorMsg, setBatchErrorMsg] = useState('')

  // Shared
  const [sending, setSending] = useState(false)
  const [rateLimitSeconds, setRateLimitSeconds] = useState(0)

  const handleSendSingle = async () => {
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

  const handleSendBatch = async () => {
    if (sending || rateLimitSeconds > 0) return
    setSending(true)
    setBatchFlash(null)
    setBatchErrorMsg('')
    setBatchResult(null)

    try {
      const result = await postEventsBatch(generateBatch(batchCount))

      if (result.ok) {
        setBatchResult(result.count)
        setBatchFlash('success')
        setTimeout(() => setBatchFlash(null), 2_000)
      } else if (result.status === 429 && 'retryAfter' in result) {
        setRateLimitSeconds(result.retryAfter)
      } else if ('message' in result) {
        setBatchErrorMsg(result.message)
        setBatchFlash('error')
        setTimeout(() => setBatchFlash(null), 3_000)
      }
    } catch {
      setBatchErrorMsg('Network error — check your connection')
      setBatchFlash('error')
      setTimeout(() => setBatchFlash(null), 3_000)
    } finally {
      setSending(false)
    }
  }

  const clearRateLimit = useCallback(() => setRateLimitSeconds(0), [])

  const tabClass = (m: Mode) =>
    [
      'px-3 py-1.5 text-xs font-medium rounded-md transition-colors',
      mode === m
        ? 'bg-zinc-700 text-zinc-100'
        : 'text-zinc-500 hover:text-zinc-300',
    ].join(' ')

  const singleBtnClass = [
    'w-full rounded-lg px-4 py-2.5 text-sm font-semibold transition-colors duration-200',
    flash === 'success'
      ? 'bg-green-500 text-white'
      : flash === 'error'
        ? 'bg-red-500 text-white'
        : sending || rateLimitSeconds > 0
          ? 'bg-cyan-600/50 text-zinc-400 cursor-not-allowed'
          : 'bg-cyan-500 text-zinc-950 hover:bg-cyan-400',
  ].join(' ')

  const batchBtnClass = [
    'w-full rounded-lg px-4 py-2.5 text-sm font-semibold transition-colors duration-200',
    batchFlash === 'success'
      ? 'bg-green-500 text-white'
      : batchFlash === 'error'
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
        {sentCount > 0 && mode === 'single' && (
          <span className="rounded-full bg-cyan-500/10 px-2.5 py-0.5 text-xs font-medium text-cyan-400">
            {sentCount} sent
          </span>
        )}
      </div>

      {/* Mode tabs */}
      <div className="flex gap-1 rounded-lg bg-zinc-800/50 p-1">
        <button type="button" className={tabClass('single')} onClick={() => setMode('single')}>
          Single
        </button>
        <button type="button" className={tabClass('batch')} onClick={() => setMode('batch')}>
          Batch
        </button>
      </div>

      {mode === 'single' ? (
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
              {EVENT_TYPES.map((t: EventType) => (
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

          <button type="button" onClick={handleSendSingle} className={singleBtnClass}>
            {sending ? 'Sending…' : flash === 'success' ? '✓ Sent' : 'Send Event'}
          </button>

          {flash === 'error' && errorMsg && (
            <p className="text-xs text-red-400" role="alert">{errorMsg}</p>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          <div className="space-y-1.5">
            <label className="text-xs text-zinc-500">Batch size</label>
            <div className="flex gap-2">
              {BATCH_COUNTS.map((n) => (
                <button
                  key={n}
                  type="button"
                  onClick={() => setBatchCount(n)}
                  className={[
                    'flex-1 rounded-lg border py-1.5 text-xs font-medium transition-colors',
                    batchCount === n
                      ? 'border-cyan-500 bg-cyan-500/10 text-cyan-400'
                      : 'border-zinc-700 text-zinc-500 hover:border-zinc-600 hover:text-zinc-300',
                  ].join(' ')}
                >
                  {n}
                </button>
              ))}
            </div>
          </div>

          <p className="text-xs text-zinc-600">
            Sends {batchCount} random events across all types with random user IDs.
          </p>

          <button type="button" onClick={handleSendBatch} className={batchBtnClass}>
            {sending
              ? 'Sending…'
              : batchFlash === 'success' && batchResult !== null
                ? `✓ ${batchResult} events queued`
                : `Send ${batchCount} Events`}
          </button>

          {batchFlash === 'error' && batchErrorMsg && (
            <p className="text-xs text-red-400" role="alert">{batchErrorMsg}</p>
          )}
        </div>
      )}

      {rateLimitSeconds > 0 && (
        <RateLimitBanner key={rateLimitSeconds} retryAfter={rateLimitSeconds} onExpire={clearRateLimit} />
      )}
    </div>
  )
}
