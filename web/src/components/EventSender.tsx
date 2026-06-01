import { useState, useCallback } from 'react'
import { postEvent, postEventsBatch, type PostEventPayload } from '@/lib/api'
import { RateLimitBanner } from './RateLimitBanner'
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

interface EventSenderProps {
  onRequest?: (entry: Omit<RequestEntry, 'id'>) => void
  onSendingChange?: (sending: boolean) => void
}

export function EventSender({ onRequest, onSendingChange }: EventSenderProps) {
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
      const result = await postEvent({ event: eventType, user_id: userId })
      const latencyMs = Date.now() - start

      if (result.ok) {
        setSentCount((c) => c + 1)
        setFlash('success')
        onRequest?.({ method: 'POST', path: '/v1/events', status: 202, latencyMs })
        setTimeout(() => setFlash(null), 1_500)
      } else if (result.status === 429 && 'retryAfter' in result) {
        setRateLimitSeconds(result.retryAfter)
        onRequest?.({ method: 'POST', path: '/v1/events', status: 429, latencyMs })
      } else if ('message' in result) {
        setErrorMsg(result.message)
        setFlash('error')
        onRequest?.({ method: 'POST', path: '/v1/events', status: result.status, latencyMs })
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
      const result = await postEventsBatch(generateBatch(batchCount))
      const latencyMs = Date.now() - start

      if (result.ok) {
        setBatchResult(result.count)
        setBatchFlash('success')
        onRequest?.({ method: 'POST', path: '/v1/events/batch', status: 202, latencyMs })
        setTimeout(() => setBatchFlash(null), 2_000)
      } else if (result.status === 429 && 'retryAfter' in result) {
        setRateLimitSeconds(result.retryAfter)
        onRequest?.({ method: 'POST', path: '/v1/events/batch', status: 429, latencyMs })
      } else if ('message' in result) {
        setBatchErrorMsg(result.message)
        setBatchFlash('error')
        onRequest?.({ method: 'POST', path: '/v1/events/batch', status: result.status, latencyMs })
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
          <TabsList className="w-full">
            <TabsTrigger value="single" className="flex-1">Single</TabsTrigger>
            <TabsTrigger value="batch" className="flex-1">Batch</TabsTrigger>
          </TabsList>

          <TabsContent value="single" className="space-y-3 mt-4">
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
                    {t}
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
              {sending ? 'Sending…' : flash === 'success' ? '✓ Sent' : 'Send Event'}
            </Button>

            {flash === 'error' && errorMsg && (
              <p className="text-xs text-destructive" role="alert">{errorMsg}</p>
            )}
          </TabsContent>

          <TabsContent value="batch" className="space-y-3 mt-4">
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
              Sends {batchCount} random events across all types with random user IDs.
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
