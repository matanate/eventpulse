import { usePolledResource } from '@/hooks/usePolledResource'
import { getQueueStats, type QueueStats } from '@/lib/api'
import { Card, CardContent } from '@/components/ui/card'

const POLL_MS = 10_000

function statusDot(value: number, warnAt: number, critAt: number) {
  if (value >= critAt) return 'bg-red-500'
  if (value >= warnAt) return 'bg-amber-400'
  return 'bg-emerald-500'
}

interface GaugeCellProps {
  label: string
  value: number | null
  dotClass: string
  description: string
}

function GaugeCell({ label, value, dotClass, description }: GaugeCellProps) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-1.5">
        <span className={`h-2 w-2 rounded-full shrink-0 ${dotClass}`} aria-hidden />
        <span className="text-xs font-medium uppercase tracking-widest text-muted-foreground">
          {label}
        </span>
      </div>
      <span className="text-2xl font-bold tabular-nums pl-3.5">
        {value !== null ? value.toLocaleString() : '—'}
      </span>
      <span className="text-[10px] text-muted-foreground/50 pl-3.5">{description}</span>
    </div>
  )
}

export function QueueHealthCard() {
  const { data, status } = usePolledResource<QueueStats>(getQueueStats, POLL_MS)

  const pendingDot = statusDot(data?.pending_messages ?? 0, 10, 50)
  const deadLetterDot = statusDot(data?.dead_letter_count ?? 0, 1, 5)

  return (
    <Card>
      <CardContent className="pt-4 pb-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-xs font-mono font-semibold uppercase tracking-widest text-muted-foreground">
            Pipeline Health
          </h2>
          {status === 'error' && (
            <span className="text-xs text-amber-500" role="status" aria-label="stale">
              stale
            </span>
          )}
        </div>
        <div className="grid grid-cols-2 gap-4">
          <GaugeCell
            label="Queue depth"
            value={data?.pending_messages ?? null}
            dotClass={pendingDot}
            description="messages pending ACK"
          />
          <GaugeCell
            label="Dead letters"
            value={data?.dead_letter_count ?? null}
            dotClass={deadLetterDot}
            description="permanently failed events"
          />
        </div>
      </CardContent>
    </Card>
  )
}
