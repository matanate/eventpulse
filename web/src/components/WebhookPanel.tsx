import { useState, useEffect, useCallback } from 'react'
import { Trash2 } from 'lucide-react'
import { createWebhook, listWebhooks, deleteWebhook, type Webhook } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import type { RequestEntry } from './RequestLog'

// Intentionally hardcoded: used only as the HMAC signing secret for demo webhooks.
// The browser bundle is public, so this is not a sensitive credential.
const DEMO_SECRET = 'eventpulse-demo-secret-key-32ch'

interface WebhookPanelProps {
  onRequest?: (entry: Omit<RequestEntry, 'id'>) => void
}

export function WebhookPanel({ onRequest }: WebhookPanelProps) {
  const [webhooks, setWebhooks] = useState<Webhook[]>([])
  const [url, setUrl] = useState('')
  const [filterEvent, setFilterEvent] = useState('')
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState('')
  const [deleteError, setDeleteError] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const list = await listWebhooks()
      setWebhooks(list)
    } catch {
      // silently — list stays empty on error
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const handleCreate = async () => {
    if (!url.trim() || creating) return
    setCreating(true)
    setCreateError('')
    const start = Date.now()
    try {
      const webhook = await createWebhook(
        url.trim(),
        DEMO_SECRET,
        filterEvent.trim() || undefined,
      )
      setWebhooks((prev) => [webhook, ...prev])
      setUrl('')
      setFilterEvent('')
      onRequest?.({
        method: 'POST',
        path: '/v1/webhooks',
        status: 201,
        latencyMs: Date.now() - start,
      })
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to register webhook'
      setCreateError(msg.replace('create webhook: ', ''))
      onRequest?.({
        method: 'POST',
        path: '/v1/webhooks',
        status: 400,
        latencyMs: Date.now() - start,
      })
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    setDeleting(id)
    const start = Date.now()
    try {
      await deleteWebhook(id)
      setWebhooks((prev) => prev.filter((w) => w.id !== id))
      onRequest?.({
        method: 'DELETE',
        path: `/v1/webhooks/${id.slice(0, 8)}…`,
        status: 204,
        latencyMs: Date.now() - start,
      })
    } catch {
      setDeleteError('Failed to delete webhook — try again')
      setTimeout(() => setDeleteError(''), 3_000)
    } finally {
      setDeleting(null)
    }
  }

  return (
    <div className="space-y-4">
      {/* Registration form */}
      <div className="space-y-2">
        <div className="space-y-1.5">
          <label className="text-xs text-muted-foreground" htmlFor="webhook-url">
            Endpoint URL
          </label>
          <Input
            id="webhook-url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') void handleCreate() }}
            placeholder="https://webhook.site/…"
            className="font-mono text-xs"
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-xs text-muted-foreground" htmlFor="webhook-filter">
            Event filter{' '}
            <span className="text-muted-foreground/50">(optional — blank = all events)</span>
          </label>
          <Input
            id="webhook-filter"
            value={filterEvent}
            onChange={(e) => setFilterEvent(e.target.value)}
            placeholder="page_viewed"
            className="font-mono text-xs"
          />
        </div>
        <Button
          type="button"
          onClick={() => void handleCreate()}
          disabled={creating || !url.trim()}
          className="w-full"
        >
          {creating ? 'Registering…' : 'Register Webhook'}
        </Button>
        {createError && (
          <p className="text-xs text-destructive" role="alert">{createError}</p>
        )}
      </div>

      {/* HMAC signing callout */}
      <div className="rounded-md border border-primary/20 bg-primary/5 px-3 py-2.5">
        <p className="text-xs leading-relaxed text-muted-foreground">
          <span className="font-medium text-primary">HMAC-SHA256 signed</span> — every delivery
          includes{' '}
          <code className="rounded bg-secondary px-1 py-0.5 font-mono text-[10px]">
            X-EventPulse-Signature: sha256=…
          </code>{' '}
          so your endpoint can verify authenticity. Use{' '}
          <a
            href="https://webhook.site"
            target="_blank"
            rel="noopener noreferrer"
            className="text-primary underline-offset-2 hover:underline"
          >
            webhook.site
          </a>{' '}
          to inspect live deliveries.
        </p>
      </div>

      {/* Registered webhooks list */}
      {deleteError && (
        <p className="text-xs text-destructive" role="alert">{deleteError}</p>
      )}

      {webhooks.length > 0 ? (
        <div className="space-y-1.5">
          <p className="text-[11px] uppercase tracking-wider text-muted-foreground/50">
            Registered ({webhooks.length})
          </p>
          <div className="space-y-1.5">
            {webhooks.map((wh) => (
              <div
                key={wh.id}
                className="flex items-center gap-2 rounded-md border border-border bg-card/40 px-3 py-2"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-xs text-foreground">{wh.url}</p>
                  {wh.filter_event && (
                    <Badge variant="outline" className="mt-1 text-[10px]">
                      {wh.filter_event}
                    </Badge>
                  )}
                </div>
                <span
                  className={`h-1.5 w-1.5 shrink-0 rounded-full ${wh.active ? 'bg-emerald-500' : 'bg-muted-foreground/30'}`}
                  title={wh.active ? 'active' : 'inactive'}
                />
                <button
                  type="button"
                  onClick={() => void handleDelete(wh.id)}
                  disabled={deleting === wh.id}
                  className="shrink-0 rounded p-1 text-muted-foreground/40 transition-colors hover:bg-destructive/10 hover:text-destructive disabled:opacity-30"
                  aria-label="Delete webhook"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <p className="text-center text-xs text-muted-foreground/50">
          No webhooks yet — register one above, then send events to trigger live deliveries
        </p>
      )}
    </div>
  )
}
