import { cn } from '@/lib/utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

interface PipelineCardProps {
  isActive: boolean
}

interface NodeProps {
  label: string
  detail?: string
  isActive: boolean
}

function PipelineNode({ label, detail, isActive }: NodeProps) {
  return (
    <div
      className={cn(
        'rounded-md border px-2.5 py-1.5 text-center transition-all duration-300',
        isActive
          ? 'border-primary/60 bg-primary/10 text-primary shadow-[0_0_8px_var(--color-primary,oklch(0.71_0.15_200))/20]'
          : 'border-border bg-card text-muted-foreground',
      )}
    >
      <div className="text-xs font-mono font-medium whitespace-nowrap">{label}</div>
      {detail && (
        <div className="text-[10px] text-muted-foreground/60 mt-0.5 whitespace-nowrap">{detail}</div>
      )}
    </div>
  )
}

function Arrow({ isActive, down = false }: { isActive: boolean; down?: boolean }) {
  return (
    <span
      className={cn(
        'text-xs transition-colors duration-300 select-none shrink-0',
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
          Pipeline architecture
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 overflow-x-auto">
        {/* Row 1 — synchronous HTTP path */}
        <div>
          <p className="text-[10px] uppercase tracking-wider text-muted-foreground/40 mb-2">
            Request path (synchronous)
          </p>
          <div className="flex items-center gap-2 flex-wrap">
            <PipelineNode label="Browser" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="Auth MW" detail="Bearer epk_…" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="Rate Limit" detail="100/min" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="Redis XADD" detail="Stream" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="+ Direct INSERT" detail="immediate" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="202 Accepted" isActive={isActive} />
          </div>
        </div>

        {/* Connector */}
        <div className={cn('flex items-center gap-1.5 text-[10px] pl-4', isActive ? 'text-primary' : 'text-muted-foreground/30')}>
          <Arrow isActive={isActive} down />
          <span className="uppercase tracking-wider">async worker picks up</span>
        </div>

        {/* Row 2 — async worker path */}
        <div>
          <p className="text-[10px] uppercase tracking-wider text-muted-foreground/40 mb-2">
            Worker (asynchronous)
          </p>
          <div className="flex items-center gap-2 flex-wrap">
            <PipelineNode label="XREADGROUP" detail="consumer group" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="Retry × 3" detail="dead letter" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="pgx INSERT" detail="events" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="UPSERT" detail="daily counts" isActive={isActive} />
            <Arrow isActive={isActive} />
            <PipelineNode label="XACK" isActive={isActive} />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
