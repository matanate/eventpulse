import { cn } from '@/lib/utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

interface PipelineCardProps {
  isActive: boolean
}

interface NodeProps {
  label: string
  detail?: string
  tooltip?: string
  isActive: boolean
}

function PipelineNode({ label, detail, tooltip, isActive }: NodeProps) {
  return (
    <div
      title={tooltip}
      className={cn(
        'rounded-md border px-2.5 py-1.5 text-center transition-all duration-300 cursor-default',
        isActive
          ? 'border-primary/60 bg-primary/10 text-primary shadow-[0_0_8px_var(--color-primary,oklch(0.71_0.15_200))/20]'
          : 'border-border bg-card text-muted-foreground',
      )}
    >
      <div className="whitespace-nowrap font-mono text-xs font-medium">{label}</div>
      {detail && (
        <div className="mt-0.5 whitespace-nowrap text-[10px] text-muted-foreground/60">{detail}</div>
      )}
    </div>
  )
}

function Arrow({ isActive, down = false }: { isActive: boolean; down?: boolean }) {
  return (
    <span
      className={cn(
        'shrink-0 select-none text-xs transition-colors duration-300',
        isActive ? 'text-primary animate-pulse' : 'text-muted-foreground/25',
        down ? 'mx-auto' : '',
      )}
    >
      {down ? '↓' : '→'}
    </span>
  )
}

export function PipelineCard({ isActive }: PipelineCardProps) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
          Pipeline Architecture
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 overflow-x-auto">
        {/* Row 1 — synchronous HTTP path */}
        <div>
          <p className="mb-2 text-[10px] uppercase tracking-wider text-muted-foreground/40">
            Request path (synchronous · &lt; 5 ms p99)
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <PipelineNode
              label="Client"
              detail="Bearer epk_…"
              tooltip="Any HTTP client — browser, SDK, or curl"
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="Auth MW"
              detail="SHA-256 lookup"
              tooltip="Hashes the bearer token, queries api_keys table, injects project_id into context"
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="Rate Limit"
              detail="100 req/min"
              tooltip="Redis sliding-window limiter keyed on api_key_id — returns 429 + Retry-After on breach"
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="Validate"
              detail="schema check"
              tooltip="JSON Schema validation against registered schemas — enforce mode returns 422, warn mode emits a metric"
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="Redis XADD"
              detail="events stream"
              tooltip="XADD events * — durable, consumer-group aware. Returns immediately after enqueue."
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="202 Accepted"
              detail="async from here"
              tooltip="Client gets 202 before the worker processes the event — fire-and-forget semantics"
              isActive={isActive}
            />
          </div>
        </div>

        {/* Connector */}
        <div
          className={cn(
            'flex items-center gap-1.5 pl-4 text-[10px]',
            isActive ? 'text-primary' : 'text-muted-foreground/30',
          )}
        >
          <Arrow isActive={isActive} down />
          <span className="uppercase tracking-wider">async worker picks up via XREADGROUP</span>
        </div>

        {/* Row 2 — async worker path */}
        <div>
          <p className="mb-2 text-[10px] uppercase tracking-wider text-muted-foreground/40">
            Worker (asynchronous · consumer group)
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <PipelineNode
              label="XREADGROUP"
              detail="consumer group"
              tooltip="Reads up to 10 messages per poll from the Redis Stream consumer group — at-least-once delivery"
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="Retry × 3"
              detail="exp. backoff"
              tooltip="Up to 3 redeliveries with exponential backoff; on 3rd failure the event moves to dead_letter_events"
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="pgx INSERT"
              detail="events table"
              tooltip="Inserts into events(project_id, event, user_id, properties, created_at) with JSONB properties"
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="UPSERT"
              detail="daily counts"
              tooltip="Upserts daily_event_counts for fast analytics aggregation without scanning the full events table"
              isActive={isActive}
            />
            <Arrow isActive={isActive} />
            <PipelineNode
              label="XACK"
              detail="message commit"
              tooltip="Acknowledges the message — removes it from the pending-entry list so it won't be redelivered"
              isActive={isActive}
            />
          </div>
        </div>

        <p className="text-[10px] text-muted-foreground/30 italic">
          Hover nodes for implementation details
        </p>
      </CardContent>
    </Card>
  )
}
