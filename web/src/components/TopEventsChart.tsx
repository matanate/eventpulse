import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Cell,
} from 'recharts'
import { usePolledResource } from '@/hooks/usePolledResource'
import { getTopEvents, type EventCount } from '@/lib/api'
import { formatEventName } from '@/lib/format'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const POLL_MS = 5_000

const chartConfig = {
  count: {
    label: 'Count',
    color: 'var(--color-chart-1)',
  },
} satisfies ChartConfig

export function TopEventsChart() {
  const { data, status } = usePolledResource<EventCount[]>(getTopEvents, POLL_MS)

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            Top Events
          </CardTitle>
          {status === 'error' && (
            <span className="text-xs text-amber-500" role="status" aria-label="stale">
              stale
            </span>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {!data || data.length === 0 ? (
          <p className="py-4 text-center text-xs text-muted-foreground">No data yet</p>
        ) : (
          <ChartContainer config={chartConfig} className="h-44 w-full">
            <BarChart
              data={data}
              layout="vertical"
              margin={{ top: 0, right: 16, bottom: 0, left: 0 }}
            >
              <XAxis
                type="number"
                tick={{ fill: 'var(--color-muted-foreground)', fontSize: 10 }}
                axisLine={false}
                tickLine={false}
              />
              <YAxis
                type="category"
                dataKey="event"
                width={140}
                tickFormatter={formatEventName}
                tick={{ fill: 'var(--color-foreground)', fontSize: 10 }}
                axisLine={false}
                tickLine={false}
              />
              <ChartTooltip content={<ChartTooltipContent />} />
              <Bar dataKey="count" radius={[0, 4, 4, 0]}>
                {data.map((_, i) => (
                  <Cell
                    key={i}
                    fill={`var(--color-chart-${Math.min(i + 1, 5)})`}
                  />
                ))}
              </Bar>
            </BarChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}
