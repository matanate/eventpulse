import { useEffect, useState } from 'react'
import { getRetention, type RetentionResult, type RetentionBucket } from '@/lib/api'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const DEMO_COHORTS = 8

type Status = 'loading' | 'ok' | 'error'

function formatCohortDate(dateStr: string): string {
  const [year, month, day] = dateStr.split('-').map(Number)
  return new Date(year, month - 1, day).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
  })
}

function CellColor({ bucket }: { bucket: RetentionBucket }) {
  const opacity = bucket.rate === 0 ? 0 : Math.max(0.12, bucket.rate)
  const pct = (bucket.rate * 100).toFixed(1)
  const label = `${bucket.count.toLocaleString()} users (${pct}%)`
  return (
    <div
      className="relative h-8 w-full overflow-hidden rounded bg-muted/40"
      title={label}
      aria-label={label}
    >
      <div
        className="absolute inset-0 rounded bg-primary"
        style={{ opacity }}
      />
      <span className="absolute inset-0 flex items-center justify-center text-[10px] tabular-nums font-medium text-foreground/80 mix-blend-normal">
        {pct}%
      </span>
    </div>
  )
}

export function RetentionGrid() {
  const [data, setData] = useState<RetentionResult | null>(null)
  const [status, setStatus] = useState<Status>('loading')

  useEffect(() => {
    let cancelled = false

    getRetention('day', DEMO_COHORTS)
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

  const maxOffset = data
    ? Math.max(0, ...data.rows.flatMap((r) => r.buckets.map((b) => b.offset)))
    : 0

  const columns = Array.from({ length: maxOffset + 1 }, (_, i) => i)

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            User Retention
          </CardTitle>
          <span className="text-xs text-muted-foreground/50">day · {DEMO_COHORTS} cohorts</span>
        </div>
      </CardHeader>
      <CardContent>
        {status === 'loading' && (
          <div className="space-y-2 py-1" aria-label="loading">
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i} className="flex gap-1">
                <div className="h-8 w-16 animate-pulse rounded bg-muted" />
                {Array.from({ length: 4 - i }).map((_, j) => (
                  <div key={j} className="h-8 flex-1 animate-pulse rounded bg-muted" />
                ))}
              </div>
            ))}
          </div>
        )}

        {status === 'error' && (
          <p className="py-4 text-center text-xs text-muted-foreground">
            Unable to load retention data
          </p>
        )}

        {status === 'ok' && data && data.rows.length === 0 && (
          <p className="py-4 text-center text-xs text-muted-foreground">No data yet</p>
        )}

        {status === 'ok' && data && data.rows.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-xs border-separate border-spacing-y-0.5">
              <thead>
                <tr>
                  <th className="w-16 pr-2 text-left font-mono text-[10px] uppercase tracking-widest text-muted-foreground/50">
                    Cohort
                  </th>
                  {columns.map((offset) => (
                    <th
                      key={offset}
                      className="px-0.5 text-center font-mono text-[10px] uppercase tracking-widest text-muted-foreground/50"
                    >
                      D+{offset}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.rows.map((row) => (
                  <tr key={row.cohort_date}>
                    <td className="pr-2 align-middle">
                      <span className="tabular-nums text-muted-foreground">
                        {formatCohortDate(row.cohort_date)}
                      </span>
                      <span className="block text-[9px] text-muted-foreground/40">
                        n={row.cohort_size}
                      </span>
                    </td>
                    {columns.map((offset) => {
                      const bucket = row.buckets.find((b) => b.offset === offset)
                      return (
                        <td key={offset} className="px-0.5">
                          {bucket ? (
                            <CellColor bucket={bucket} />
                          ) : (
                            <div className="h-8 w-full rounded bg-muted/10" />
                          )}
                        </td>
                      )
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
