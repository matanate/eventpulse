import { useCallback } from 'react'
import { RefreshCw } from 'lucide-react'
import { usePolledResource } from '@/hooks/usePolledResource'
import { postFunnel, type FunnelResult } from '@/lib/api'
import { formatEventName } from '@/lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

const DEMO_STEPS = ['page_viewed', 'button_clicked', 'checkout_completed']
const DEMO_WINDOW = 'P7D'
const POLL_MS = 30_000

export function FunnelChart() {
  const fetcher = useCallback(() => postFunnel(DEMO_STEPS, DEMO_WINDOW), [])
  const { data, status, lastUpdated, refetch } = usePolledResource<FunnelResult>(fetcher, POLL_MS)

  const maxEntered = data?.steps[0]?.entered ?? 1
  const isLoading = status === 'idle'

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
              Funnel Analysis
            </CardTitle>
            {lastUpdated && (
              <p className="mt-0.5 text-[10px] text-muted-foreground/40">
                Updated {lastUpdated.toLocaleTimeString()}
              </p>
            )}
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground/50">{DEMO_WINDOW}</span>
            <button
              type="button"
              onClick={refetch}
              disabled={isLoading}
              aria-label="Refresh funnel data"
              className="rounded p-1 text-muted-foreground/40 transition-colors hover:bg-secondary hover:text-muted-foreground disabled:opacity-30"
            >
              <RefreshCw className={cn('h-3 w-3', isLoading && 'animate-spin')} />
            </button>
            {status === 'error' && (
              <span className="text-xs text-amber-500" role="status">stale</span>
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading && (
          <div className="space-y-3 py-1" aria-label="loading">
            {DEMO_STEPS.map((s) => (
              <div key={s} className="space-y-1">
                <div className="h-2 w-24 animate-pulse rounded bg-muted" />
                <div className="h-6 animate-pulse rounded bg-muted" />
              </div>
            ))}
          </div>
        )}

        {!isLoading && status === 'error' && !data && (
          <p className="py-4 text-center text-xs text-muted-foreground">
            Unable to load funnel data
          </p>
        )}

        {data && data.steps.length === 0 && (
          <p className="py-4 text-center text-xs text-muted-foreground">No data yet</p>
        )}

        {data && data.steps.length > 0 && (
          <div className="space-y-1">
            {data.steps.map((step, i) => {
              const widthPct = maxEntered > 0 ? (step.entered / maxEntered) * 100 : 0
              const isLast = i === data.steps.length - 1

              return (
                <div key={step.event}>
                  <div className="mb-0.5 flex items-center justify-between text-xs">
                    <span className="font-medium text-foreground">
                      {formatEventName(step.event)}
                    </span>
                    <span className="tabular-nums text-muted-foreground">
                      {step.entered.toLocaleString()}
                    </span>
                  </div>
                  <div className="relative h-6 overflow-hidden rounded bg-muted/40">
                    <div
                      className="h-full rounded bg-primary/80 transition-all duration-500"
                      style={{ width: `${widthPct}%` }}
                      role="progressbar"
                      aria-label={`${formatEventName(step.event)}: ${step.entered}`}
                      aria-valuenow={step.entered}
                      aria-valuemin={0}
                      aria-valuemax={maxEntered}
                    />
                  </div>

                  {!isLast && (
                    <div className="mb-1 mt-1 flex items-center gap-1 text-xs text-muted-foreground/60">
                      <span className="text-[10px]">↓</span>
                      <span className="tabular-nums">
                        {(step.conversion_rate * 100).toFixed(1)}% continue
                      </span>
                      <span className="text-muted-foreground/40">·</span>
                      <span className="tabular-nums text-muted-foreground/40">
                        {step.dropped.toLocaleString()} dropped
                      </span>
                    </div>
                  )}
                </div>
              )
            })}

            <div className="mt-3 flex items-center justify-between border-t border-border pt-2 text-xs text-muted-foreground">
              <span>Overall conversion</span>
              <span className="font-semibold tabular-nums text-foreground">
                {(data.overall_conversion_rate * 100).toFixed(1)}%
              </span>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
