import { useState, useCallback, useEffect, useRef } from 'react'
import { EventPulseClient } from '@eventpulse/client'
import { postEventWithBadKey, postEventDuplicate, type PostEventPayload } from '@/lib/api'
import { API_BASE_URL, DEMO_API_KEY } from '@/lib/constants'
import { formatEventName } from '@/lib/format'
import { RateLimitBanner } from './RateLimitBanner'
import { WebhookPanel } from './WebhookPanel'
import { SchemaPanel } from './SchemaPanel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { RequestEntry } from './RequestLog'

const EVENT_TYPES = [
  'page_viewed',
  'user_signed_up',
  'checkout_completed',
  'button_clicked',
  'login_failed',
] as const

type EventType = (typeof EVENT_TYPES)[number]
type FlashState = 'success' | 'error' | 'idempotent' | null

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

interface EventSenderProps {
  onRequest?: (entry: Omit<RequestEntry, 'id'>) => void
  onSendingChange?: (sending: boolean) => void
}

export function EventSender({ onRequest, onSendingChange }: EventSenderProps) {
  const clientRef = useRef<EventPulseClient | null>(null)
  if (clientRef.current === null) {
    clientRef.current = new EventPulseClient({
      endpoint: API_BASE_URL,
      apiKey: DEMO_API_KEY,
      flushIntervalMs: 60_000,
      maxQueueSize: 100,
      maxRetries: 3,
    })
  }
  const client = clientRef.current

  useEffect(() => () => client.destroy(), [client])

  const [eventType, setEventType] = useState<string>(EVENT_TYPES[0])
  const [userId, setUserId] = useState(randomUserId)
  const [sentCount, setSentCount] = useState(0)
  const [flash, setFlash] = useState<FlashState>(null)
  const [errorMsg, setErrorMsg] = useState('')

  const [batchCount, setBatchCount] = useState<number>(10)
  const [batchFlash, setBatchFlash] = useState<FlashState>(null)
  const [batchResult, setBatchResult] = useState<number | null>(null)
  const [batchErrorMsg, setBatchErrorMsg] = useState('')

  const [sending, setSending] = useState(false)
  const [rateLimitSeconds, setRateLimitSeconds] = useState(0)

  function setSendingState(v: boolean) {
    setSending(v)
    onSendingChange?.(v)
  }

  const handleSendSingle = async () => {
    if (sending || rateLimitSeconds > 0) return
    setSendingState(true)
    setFlash(null)
    setErrorMsg('')

    const start = Date.now()
    try {
      client.track(eventType, userId)
      const result = await client.flush()
      const latencyMs = Date.now() - start

      if (result.sent > 0) {
        setSentCount((c) => c + 1)
        setFlash('success')
        onRequest?.({ method: 'POST', path: '/v1/events', status: 202, latencyMs })
        setTimeout(() => setFlash(null), 1_500)
      } else if (result.rateLimitSeconds !== undefined) {
        setRateLimitSeconds(result.rateLimitSeconds)
        onRequest?.({ method: 'POST', path: '/v1/events', status: 429, latencyMs })
      } else {
        setErrorMsg('Failed to send — server error')
        setFlash('error')
        onRequest?.({ method: 'POST', path: '/v1/events', status: 500, latencyMs })
        setTimeout(() => setFlash(null), 3_000)
      }
    } catch {
      const latencyMs = Date.now() - start
      setErrorMsg('Network error — check your connection')
      setFlash('error')
      onRequest?.({ method: 'POST', path: '/v1/events', status: 0, latencyMs })
      setTimeout(() => setFlash(null), 3_000)
    } finally {
      setSendingState(false)
    }
  }

  const handleSendBatch = async () => {
    if (sending || rateLimitSeconds > 0) return
    setSendingState(true)
    setBatchFlash(null)
    setBatchErrorMsg('')
    setBatchResult(null)

    const start = Date.now()
    try {
      const payload = generateBatch(batchCount)
      const result = await client.trackBatch(
        payload.map((e) => ({ event: e.event, userId: e.user_id, properties: e.properties })),
      )
      const latencyMs = Date.now() - start

      if (result.sent > 0) {
        setSentCount((c) => c + result.sent)
        setBatchResult(result.sent)
        setBatchFlash('success')
        onRequest?.({ method: 'POST', path: '/v1/events/batch', status: 202, latencyMs })
        setTimeout(() => setBatchFlash(null), 2_000)
      } else if (result.rateLimitSeconds !== undefined) {
        setRateLimitSeconds(result.rateLimitSeconds)
        onRequest?.({ method: 'POST', path: '/v1/events/batch', status: 429, latencyMs })
      } else {
        setBatchErrorMsg('Failed to send — server error')
        setBatchFlash('error')
        onRequest?.({ method: 'POST', path: '/v1/events/batch', status: 500, latencyMs })
        setTimeout(() => setBatchFlash(null), 3_000)
      }
    } catch {
      const latencyMs = Date.now() - start
      setBatchErrorMsg('Network error — check your connection')
      setBatchFlash('error')
      onRequest?.({ method: 'POST', path: '/v1/events/batch', status: 0, latencyMs })
      setTimeout(() => setBatchFlash(null), 3_000)
    } finally {
      setSendingState(false)
    }
  }

  const handleDuplicateDemo = async () => {
    if (sending) return
    setSendingState(true)
    setFlash(null)
    const start = Date.now()
    try {
      const { first, second } = await postEventDuplicate({ event: eventType, user_id: userId })
      const latencyMs = Date.now() - start
      const firstStatus = first.ok ? 202 : (first.status as number)
      const secondStatus = second.ok ? 202 : (second.status as number)
      onRequest?.({ method: 'POST', path: '/v1/events (×1)', status: firstStatus, latencyMs })
      onRequest?.({ method: 'POST', path: '/v1/events (×2 dup)', status: secondStatus, latencyMs: 0 })
      if (first.ok && second.ok) {
        setFlash('idempotent')
        setTimeout(() => setFlash(null), 3_000)
      }
    } catch {
      const latencyMs = Date.now() - start
      onRequest?.({ method: 'POST', path: '/v1/events', status: 0, latencyMs })
    } finally {
      setSendingState(false)
    }
  }

  const handleAuthFailureDemo = async () => {
    if (sending) return
    setSendingState(true)
    const start = Date.now()
    try {
      const result = await postEventWithBadKey({ event: eventType, user_id: userId })
      const latencyMs = Date.now() - start
      onRequest?.({ method: 'POST', path: '/v1/events', status: result.ok ? 202 : result.status, latencyMs })
    } catch {
      const latencyMs = Date.now() - start
      onRequest?.({ method: 'POST', path: '/v1/events', status: 401, latencyMs })
    } finally {
      setSendingState(false)
    }
  }

  const clearRateLimit = useCallback(() => setRateLimitSeconds(0), [])

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            Send Event
          </CardTitle>
          {sentCount > 0 && (
            <Badge variant="secondary" className="text-[10px]">
              {sentCount} sent
            </Badge>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <Tabs defaultValue="single">
          <TabsList className="grid w-full grid-cols-4">
            <TabsTrigger value="single" className="text-xs">Single</TabsTrigger>
            <TabsTrigger value="batch" className="text-xs">Batch</TabsTrigger>
            <TabsTrigger value="webhooks" className="text-xs">Webhooks</TabsTrigger>
            <TabsTrigger value="schema" className="text-xs">Schema</TabsTrigger>
          </TabsList>

          {/* ── Single event ── */}
          <TabsContent value="single" className="mt-4 space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-muted-foreground" htmlFor="event-type">
                Event type
              </label>
              <select
                id="event-type"
                value={eventType}
                onChange={(e) => setEventType(e.target.value)}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus:border-primary focus:outline-none"
              >
                {EVENT_TYPES.map((t: EventType) => (
                  <option key={t} value={t}>
                    {formatEventName(t)}
                  </option>
                ))}
              </select>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs text-muted-foreground" htmlFor="user-id">
                User ID
              </label>
              <Input
                id="user-id"
                value={userId}
                onChange={(e) => setUserId(e.target.value)}
                placeholder="user_123"
              />
            </div>

            <Button
              type="button"
              onClick={() => { void handleSendSingle() }}
              disabled={sending || rateLimitSeconds > 0}
              variant={flash === 'error' ? 'destructive' : 'default'}
              className="w-full"
            >
              {sending
                ? 'Sending…'
                : flash === 'success'
                  ? '✓ Sent'
                  : flash === 'idempotent'
                    ? '✓ Sent (×2)'
                    : 'Send Event'}
            </Button>

            {flash === 'error' && errorMsg && (
              <p className="text-xs text-destructive" role="alert">{errorMsg}</p>
            )}
            {flash === 'idempotent' && (
              <p className="text-xs text-emerald-400" role="status">
                2× accepted · 1 unique event (idempotency proof)
              </p>
            )}

            <div className="flex flex-col gap-1">
              <button
                type="button"
                onClick={() => { void handleDuplicateDemo() }}
                disabled={sending}
                className="w-full text-center text-[11px] text-muted-foreground/40 transition-colors hover:text-muted-foreground/70 disabled:pointer-events-none"
              >
                Demo: send duplicate (idempotency) →
              </button>
              <button
                type="button"
                onClick={() => { void handleAuthFailureDemo() }}
                disabled={sending}
                className="w-full text-center text-[11px] text-muted-foreground/40 transition-colors hover:text-muted-foreground/70 disabled:pointer-events-none"
              >
                Demo: try invalid API key →
              </button>
            </div>
          </TabsContent>

          {/* ── Batch events ── */}
          <TabsContent value="batch" className="mt-4 space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-muted-foreground">Batch size</label>
              <div className="flex gap-2">
                {BATCH_COUNTS.map((n) => (
                  <Button
                    key={n}
                    type="button"
                    size="sm"
                    variant={batchCount === n ? 'default' : 'outline'}
                    onClick={() => setBatchCount(n)}
                    className="flex-1"
                  >
                    {n}
                  </Button>
                ))}
              </div>
            </div>

            <p className="text-xs text-muted-foreground">
              Sends {batchCount} random events across all types with random user IDs via the SDK's
              batch endpoint.
            </p>

            <Button
              type="button"
              onClick={() => { void handleSendBatch() }}
              disabled={sending || rateLimitSeconds > 0}
              variant={batchFlash === 'error' ? 'destructive' : 'default'}
              className={cn('w-full', batchFlash === 'success' && 'bg-green-600 hover:bg-green-600')}
            >
              {sending
                ? 'Sending…'
                : batchFlash === 'success' && batchResult !== null
                  ? `✓ ${batchResult} events queued`
                  : `Send ${batchCount} Events`}
            </Button>

            {batchFlash === 'error' && batchErrorMsg && (
              <p className="text-xs text-destructive" role="alert">{batchErrorMsg}</p>
            )}
          </TabsContent>

          {/* ── Webhooks ── */}
          <TabsContent value="webhooks" className="mt-4">
            <WebhookPanel onRequest={onRequest} />
          </TabsContent>

          {/* ── Schema registry ── */}
          <TabsContent value="schema" className="mt-4">
            <SchemaPanel onRequest={onRequest} />
          </TabsContent>
        </Tabs>

        {rateLimitSeconds > 0 && (
          <RateLimitBanner
            key={rateLimitSeconds}
            retryAfter={rateLimitSeconds}
            onExpire={clearRateLimit}
          />
        )}
      </CardContent>
    </Card>
  )
}
