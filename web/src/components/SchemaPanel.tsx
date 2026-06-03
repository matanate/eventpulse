import { useState, useEffect, useCallback } from 'react'
import { Trash2 } from 'lucide-react'
import {
  upsertSchema,
  listSchemas,
  deleteSchema,
  postEventWithProperties,
  type EventSchema,
  type SchemaMode,
} from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { RequestEntry } from './RequestLog'

const EVENT_TYPES = [
  'page_viewed',
  'user_signed_up',
  'checkout_completed',
  'button_clicked',
  'login_failed',
]

const DEFAULT_SCHEMA = JSON.stringify(
  {
    type: 'object',
    properties: {
      source: { type: 'string' },
    },
    required: ['source'],
  },
  null,
  2,
)

type TestResult = { status: 202 | 422; message: string } | null

interface SchemaPanelProps {
  onRequest?: (entry: Omit<RequestEntry, 'id'>) => void
}

export function SchemaPanel({ onRequest }: SchemaPanelProps) {
  const [schemas, setSchemas] = useState<EventSchema[]>([])
  const [eventName, setEventName] = useState(EVENT_TYPES[0])
  const [schemaJson, setSchemaJson] = useState(DEFAULT_SCHEMA)
  const [mode, setMode] = useState<SchemaMode>('warn')
  const [registering, setRegistering] = useState(false)
  const [registerError, setRegisterError] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<TestResult>(null)
  const [testing, setTesting] = useState(false)

  const load = useCallback(async () => {
    try {
      const list = await listSchemas()
      setSchemas(list)
    } catch {
      // silently — list stays empty
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const handleRegister = async () => {
    if (!eventName || registering) return
    setRegistering(true)
    setRegisterError('')
    const start = Date.now()
    try {
      const parsed = JSON.parse(schemaJson) as Record<string, unknown>
      const schema = await upsertSchema(eventName, parsed, mode)
      setSchemas((prev) => {
        const without = prev.filter((s) => s.event_name !== eventName)
        return [schema, ...without]
      })
      onRequest?.({
        method: 'POST',
        path: `/v1/projects/…/schemas/${eventName}`,
        status: 201,
        latencyMs: Date.now() - start,
      })
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to register schema'
      setRegisterError(msg.replace('upsert schema: ', ''))
      onRequest?.({
        method: 'POST',
        path: `/v1/projects/…/schemas/${eventName}`,
        status: 400,
        latencyMs: Date.now() - start,
      })
    } finally {
      setRegistering(false)
    }
  }

  const handleDelete = async (name: string) => {
    setDeleting(name)
    const start = Date.now()
    try {
      await deleteSchema(name)
      setSchemas((prev) => prev.filter((s) => s.event_name !== name))
      onRequest?.({
        method: 'DELETE',
        path: `/v1/projects/…/schemas/${name}`,
        status: 204,
        latencyMs: Date.now() - start,
      })
    } catch {
      // silently ignore
    } finally {
      setDeleting(null)
    }
  }

  const handleTestViolation = async (schema: EventSchema) => {
    if (testing) return
    setTesting(true)
    setTestResult(null)
    const start = Date.now()
    // Send properties that deliberately omit the required "source" field
    const badProperties = { intentionally_invalid: true, missing_required_fields: true }
    try {
      const result = await postEventWithProperties(schema.event_name, badProperties)
      const latencyMs = Date.now() - start
      if (result.status === 422 && 'violations' in result) {
        setTestResult({ status: 422, message: `422 — ${result.violations.join('; ')}` })
        onRequest?.({
          method: 'POST',
          path: `/v1/events (schema ${schema.mode})`,
          status: 422,
          latencyMs,
        })
      } else if (result.status === 202) {
        setTestResult({
          status: 202,
          message: `202 accepted (warn mode — violation logged as metric)`,
        })
        onRequest?.({
          method: 'POST',
          path: `/v1/events (schema ${schema.mode})`,
          status: 202,
          latencyMs,
        })
      } else {
        setTestResult({ status: 202, message: `${result.status} — unexpected response` })
      }
    } catch {
      setTestResult({ status: 202, message: 'Network error during test' })
    } finally {
      setTesting(false)
      setTimeout(() => setTestResult(null), 5_000)
    }
  }

  return (
    <div className="space-y-4">
      {/* Registration form */}
      <div className="space-y-2">
        <div className="space-y-1.5">
          <label className="text-xs text-muted-foreground" htmlFor="schema-event">
            Event type
          </label>
          <select
            id="schema-event"
            value={eventName}
            onChange={(e) => setEventName(e.target.value)}
            className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus:border-primary focus:outline-none"
          >
            {EVENT_TYPES.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs text-muted-foreground" htmlFor="schema-json">
            JSON Schema
          </label>
          <textarea
            id="schema-json"
            value={schemaJson}
            onChange={(e) => setSchemaJson(e.target.value)}
            rows={6}
            className="w-full resize-none rounded-md border border-input bg-background px-3 py-2 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
            spellCheck={false}
          />
        </div>

        <div className="space-y-1.5">
          <p className="text-xs text-muted-foreground">Validation mode</p>
          <div className="flex gap-2">
            {(['warn', 'enforce'] as const).map((m) => (
              <button
                key={m}
                type="button"
                onClick={() => setMode(m)}
                className={cn(
                  'flex-1 rounded-md border px-3 py-1.5 text-xs font-medium transition-all',
                  mode === m
                    ? m === 'enforce'
                      ? 'border-destructive/60 bg-destructive/10 text-destructive'
                      : 'border-amber-500/60 bg-amber-500/10 text-amber-400'
                    : 'border-border text-muted-foreground hover:border-border/80',
                )}
              >
                {m === 'enforce' ? '⚡ enforce (422)' : '⚠ warn (202)'}
              </button>
            ))}
          </div>
          <p className="text-[10px] text-muted-foreground/50">
            {mode === 'enforce'
              ? 'Non-conforming events are rejected with 422 Unprocessable Entity'
              : 'Non-conforming events are accepted but a schema_violations_total metric is emitted'}
          </p>
        </div>

        <Button
          type="button"
          onClick={() => void handleRegister()}
          disabled={registering}
          className="w-full"
        >
          {registering ? 'Registering…' : 'Register Schema'}
        </Button>
        {registerError && (
          <p className="text-xs text-destructive" role="alert">{registerError}</p>
        )}
      </div>

      {/* Registered schemas */}
      {schemas.length > 0 ? (
        <div className="space-y-1.5">
          <p className="text-[11px] uppercase tracking-wider text-muted-foreground/50">
            Registered schemas ({schemas.length})
          </p>
          <div className="space-y-2">
            {schemas.map((schema) => (
              <div
                key={schema.event_name}
                className="rounded-md border border-border bg-card/40 px-3 py-2.5"
              >
                <div className="flex items-center gap-2">
                  <span className="flex-1 font-mono text-xs text-foreground">
                    {schema.event_name}
                  </span>
                  <Badge
                    variant="outline"
                    className={cn(
                      'text-[10px] shrink-0',
                      schema.mode === 'enforce'
                        ? 'border-destructive/40 text-destructive'
                        : 'border-amber-500/40 text-amber-400',
                    )}
                  >
                    {schema.mode}
                  </Badge>
                  <button
                    type="button"
                    onClick={() => void handleDelete(schema.event_name)}
                    disabled={deleting === schema.event_name}
                    className="shrink-0 rounded p-1 text-muted-foreground/40 transition-colors hover:bg-destructive/10 hover:text-destructive disabled:opacity-30"
                    aria-label="Delete schema"
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>

                {/* Test violation button */}
                <div className="mt-2 flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => void handleTestViolation(schema)}
                    disabled={testing}
                    className="text-[11px] text-muted-foreground/50 transition-colors hover:text-primary disabled:opacity-30"
                  >
                    {testing ? 'Sending…' : 'Demo: send a violating event →'}
                  </button>
                </div>

                {testResult && (
                  <p
                    className={cn(
                      'mt-1.5 text-[11px] font-mono',
                      testResult.status === 422 ? 'text-destructive' : 'text-amber-400',
                    )}
                    role="status"
                  >
                    {testResult.message}
                  </p>
                )}
              </div>
            ))}
          </div>
        </div>
      ) : (
        <p className="text-center text-xs text-muted-foreground/50">
          No schemas registered — add one above, then send an event to see validation in action
        </p>
      )}
    </div>
  )
}
