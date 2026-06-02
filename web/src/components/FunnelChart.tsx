import { useEffect, useState } from 'react'
import { postFunnel, type FunnelResult } from '@/lib/api'
import { formatEventName } from '@/lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const DEMO_STEPS = ['page_viewed', 'button_clicked', 'checkout_completed']
const DEMO_WINDOW = 'P7D'

type Status = 'loading' | 'ok' | 'error'

export function FunnelChart() {
  const [data, setData] = useState<FunnelResult | null>(null)
  const [status, setStatus] = useState<Status>('loading')

  useEffect(() => {
    let cancelled = false

    postFunnel(DEMO_STEPS, DEMO_WINDOW)
      .then((result) => {
        if (!cancelled) {
          setData(result)
          setStatus('ok')
        }
      })
      .catch(() => {
        if (!cancelled) setStatus('error')
      })

    return () => {
      cancelled = true
    }
  }, [])

  const maxEntered = data?.steps[0]?.entered ?? 1

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            Funnel Analysis
          </CardTitle>
          <span className="text-xs text-muted-foreground/50">{DEMO_WINDOW}</span>
        </div>
      </CardHeader>
      <CardContent>
        {status === 'loading' && (
          <div className="space-y-3 py-1" aria-label="loading">
            {DEMO_STEPS.map((s) => (
              <div key={s} className="space-y-1">
                <div className="h-2 w-24 animate-pulse rounded bg-muted" />
                <div className="h-6 animate-pulse rounded bg-muted" />
              </div>
            ))}
          </div>
        )}

        {status === 'error' && (
          <p className="py-4 text-center text-xs text-muted-foreground">
            Unable to load funnel data
          </p>
        )}

        {status === 'ok' && data && data.steps.length === 0 && (
          <p className="py-4 text-center text-xs text-muted-foreground">No data yet</p>
        )}

        {status === 'ok' && data && data.steps.length > 0 && (
          <div className="space-y-1">
            {data.steps.map((step, i) => {
              const widthPct = maxEntered > 0 ? (step.entered / maxEntered) * 100 : 0
              const isLast = i === data.steps.length - 1

              return (
                <div key={step.event}>
                  {/* Step bar */}
                  <div className="mb-0.5 flex items-center justify-between text-xs">
                    <span className="font-medium text-foreground">
                      {formatEventName(step.event)}
                    </span>
                    <span className="tabular-nums text-muted-foreground">{step.entered.toLocaleString()}</span>
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

                  {/* Drop-off arrow between steps */}
                  {!isLast && (
                    <div className="mt-1 mb-1 flex items-center gap-1 text-xs text-muted-foreground/60">
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

            {/* Overall rate footer */}
            <div className="mt-3 border-t border-border pt-2 flex items-center justify-between text-xs text-muted-foreground">
              <span>Overall conversion</span>
              <span className="tabular-nums font-semibold text-foreground">
                {(data.overall_conversion_rate * 100).toFixed(1)}%
              </span>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
