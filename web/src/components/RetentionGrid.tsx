import { useCallback } from 'react'
import { RefreshCw } from 'lucide-react'
import { usePolledResource } from '@/hooks/usePolledResource'
import { getRetention, type RetentionResult, type RetentionBucket } from '@/lib/api'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

const DEMO_COHORTS = 8
const POLL_MS = 60_000

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
      <div className="absolute inset-0 rounded bg-primary" style={{ opacity }} />
      <span className="absolute inset-0 flex items-center justify-center text-[10px] tabular-nums font-medium text-foreground/80">
        {pct}%
      </span>
    </div>
  )
}

export function RetentionGrid() {
  const fetcher = useCallback(() => getRetention('day', DEMO_COHORTS), [])
  const { data, status, lastUpdated, refetch } = usePolledResource<RetentionResult>(fetcher, POLL_MS)

  const isLoading = status === 'idle'
  const maxOffset = data
    ? Math.max(0, ...data.rows.flatMap((r) => r.buckets.map((b) => b.offset)))
    : 0
  const columns = Array.from({ length: maxOffset + 1 }, (_, i) => i)

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
              User Retention
            </CardTitle>
            {lastUpdated && (
              <p className="mt-0.5 text-[10px] text-muted-foreground/40">
                Updated {lastUpdated.toLocaleTimeString()}
              </p>
            )}
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground/50">day · {DEMO_COHORTS} cohorts</span>
            <button
              type="button"
              onClick={refetch}
              disabled={isLoading}
              aria-label="Refresh retention data"
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

        {!isLoading && status === 'error' && !data && (
          <p className="py-4 text-center text-xs text-muted-foreground">
            Unable to load retention data
          </p>
        )}

        {data && data.rows.length === 0 && (
          <p className="py-4 text-center text-xs text-muted-foreground">No data yet</p>
        )}

        {data && data.rows.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full border-separate border-spacing-y-0.5 text-xs">
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
