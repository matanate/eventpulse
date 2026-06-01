import { useMemo } from 'react'
import { AreaChart, Area, XAxis, YAxis } from 'recharts'
import { usePolledResource } from '@/hooks/usePolledResource'
import { listEvents, type EventsResponse } from '@/lib/api'
import {
  ChartContainer,
  ChartTooltip,
  type ChartConfig,
} from '@/components/ui/chart'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const POLL_MS = 5_000
const BUCKET_MINUTES = 5
const BUCKET_COUNT = 12

const chartConfig = {
  count: { label: 'Events', color: 'var(--color-chart-1)' },
} satisfies ChartConfig

interface Bucket {
  label: string
  count: number
  startMs: number
}

function buildBuckets(nowMs: number): Bucket[] {
  return Array.from({ length: BUCKET_COUNT }, (_, i) => {
    const bucketIndex = BUCKET_COUNT - 1 - i
    const startMs = nowMs - (bucketIndex + 1) * BUCKET_MINUTES * 60_000
    const minutesAgo = (bucketIndex + 1) * BUCKET_MINUTES
    const label = minutesAgo === 0 ? 'now' : `${minutesAgo}m`
    return { label, count: 0, startMs }
  })
}

function bucketEvents(events: EventsResponse['events']): Bucket[] {
  const now = Date.now()
  const buckets = buildBuckets(now)
  const windowStart = buckets[0]!.startMs

  for (const evt of events) {
    const ts = new Date(evt.timestamp).getTime()
    if (ts < windowStart) continue
    const index = Math.min(
      BUCKET_COUNT - 1,
      Math.floor((ts - windowStart) / (BUCKET_MINUTES * 60_000)),
    )
    buckets[index]!.count++
  }

  return buckets
}

interface TooltipPayloadItem {
  value: number
}

interface CustomTooltipProps {
  active?: boolean
  payload?: TooltipPayloadItem[]
  label?: string
}

function CustomTooltip({ active, payload, label }: CustomTooltipProps) {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md">
      <span className="text-muted-foreground">{label}: </span>
      <span className="font-medium">{payload[0]!.value} events</span>
    </div>
  )
}

export function ActivityTimeline() {
  const { data, status } = usePolledResource<EventsResponse>(
    () => listEvents({ limit: 200 }),
    POLL_MS,
  )

  const buckets = useMemo(
    () => bucketEvents(data?.events ?? []),
    [data],
  )

  const maxCount = Math.max(...buckets.map((b) => b.count), 1)

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            Ingestion Rate
          </CardTitle>
          <span className="text-[10px] text-muted-foreground/40 tabular-nums">
            5-min · last hour
          </span>
          {status === 'error' && (
            <span className="text-xs text-amber-500" role="status" aria-label="stale">stale</span>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <ChartContainer
          config={chartConfig}
          className="h-28 w-full"
          aria-label="Event ingestion rate over time"
        >
          <AreaChart data={buckets} margin={{ top: 4, right: 4, bottom: 0, left: -20 }}>
            <defs>
              <linearGradient id="timelineGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-chart-1)" stopOpacity={0.35} />
                <stop offset="95%" stopColor="var(--color-chart-1)" stopOpacity={0.02} />
              </linearGradient>
            </defs>
            <XAxis
              dataKey="label"
              tick={{ fill: 'var(--color-muted-foreground)', fontSize: 9 }}
              axisLine={false}
              tickLine={false}
              interval={2}
            />
            <YAxis
              domain={[0, maxCount + 1]}
              allowDecimals={false}
              tick={{ fill: 'var(--color-muted-foreground)', fontSize: 9 }}
              axisLine={false}
              tickLine={false}
              tickCount={3}
            />
            <ChartTooltip content={<CustomTooltip />} />
            <Area
              type="monotone"
              dataKey="count"
              stroke="var(--color-chart-1)"
              strokeWidth={1.5}
              fill="url(#timelineGradient)"
              dot={false}
              activeDot={{ r: 3, fill: 'var(--color-chart-1)', strokeWidth: 0 }}
            />
          </AreaChart>
        </ChartContainer>
      </CardContent>
    </Card>
  )
}
