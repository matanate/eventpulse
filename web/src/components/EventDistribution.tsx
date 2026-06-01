import { useMemo } from 'react'
import { PieChart, Pie, Cell } from 'recharts'
import { usePolledResource } from '@/hooks/usePolledResource'
import { listEvents, type EventsResponse } from '@/lib/api'
import { formatEventName } from '@/lib/format'
import { eventColor } from '@/lib/eventColors'
import {
  ChartContainer,
  ChartTooltip,
  type ChartConfig,
} from '@/components/ui/chart'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const POLL_MS = 5_000

const chartConfig = {} satisfies ChartConfig

interface DistEntry {
  event: string
  count: number
  pct: number
  color: string
}

function computeDistribution(events: EventsResponse['events']): DistEntry[] {
  const counts: Record<string, number> = {}
  for (const e of events) {
    counts[e.event] = (counts[e.event] ?? 0) + 1
  }
  const total = events.length
  if (total === 0) return []
  return Object.entries(counts)
    .sort(([, a], [, b]) => b - a)
    .map(([event, count], i) => ({
      event,
      count,
      pct: Math.round((count / total) * 100),
      color: eventColor(event, i),
    }))
}

interface TooltipPayloadItem {
  payload: DistEntry
}

interface CustomTooltipProps {
  active?: boolean
  payload?: TooltipPayloadItem[]
}

function CustomTooltip({ active, payload }: CustomTooltipProps) {
  if (!active || !payload?.length) return null
  const d = payload[0]!.payload
  return (
    <div className="rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md">
      <span className="font-medium">{formatEventName(d.event)}</span>
      <span className="ml-2 text-muted-foreground">{d.count} ({d.pct}%)</span>
    </div>
  )
}

export function EventDistribution() {
  const { data, status } = usePolledResource<EventsResponse>(
    () => listEvents({ limit: 200 }),
    POLL_MS,
  )

  const dist = useMemo(
    () => computeDistribution(data?.events ?? []),
    [data],
  )

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            Event Distribution
          </CardTitle>
          {status === 'error' && (
            <span className="text-xs text-amber-500" role="status" aria-label="stale">stale</span>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {dist.length === 0 ? (
          <p className="py-4 text-center text-xs text-muted-foreground">No data yet</p>
        ) : (
          <div className="flex items-center gap-4">
            <ChartContainer
              config={chartConfig}
              className="h-36 w-36 shrink-0 aspect-square!"
              aria-label="Event distribution donut chart"
            >
              <PieChart>
                <Pie
                  data={dist}
                  dataKey="count"
                  nameKey="event"
                  cx="50%"
                  cy="50%"
                  innerRadius="55%"
                  outerRadius="80%"
                  strokeWidth={2}
                  stroke="var(--color-background)"
                >
                  {dist.map((d) => (
                    <Cell key={d.event} fill={d.color} />
                  ))}
                </Pie>
                <ChartTooltip content={<CustomTooltip />} />
              </PieChart>
            </ChartContainer>

            <div className="flex-1 space-y-1.5 min-w-0">
              {dist.map((d) => (
                <div key={d.event} className="flex items-center gap-2 min-w-0">
                  <span
                    className="h-2 w-2 shrink-0 rounded-full"
                    style={{ backgroundColor: d.color }}
                  />
                  <span className="flex-1 truncate text-xs text-foreground">
                    {formatEventName(d.event)}
                  </span>
                  <span className="shrink-0 tabular-nums text-xs text-muted-foreground">
                    {d.pct}%
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
