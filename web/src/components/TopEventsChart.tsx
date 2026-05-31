import { useEffect, useState } from 'react'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from 'recharts'
import { getTopEvents, type EventCount } from '../lib/api'

const POLL_MS = 5_000
const BAR_COLORS = ['#06b6d4', '#0891b2', '#0e7490', '#155e75', '#164e63']

export function TopEventsChart() {
  const [data, setData] = useState<EventCount[]>([])

  useEffect(() => {
    let cancelled = false

    const poll = async () => {
      try {
        const result = await getTopEvents(5)
        if (!cancelled) setData(result)
      } catch {
        // preserve last known state
      }
    }

    poll()
    const id = setInterval(poll, POLL_MS)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])

  return (
    <div className="space-y-3">
      <h2 className="text-sm font-semibold uppercase tracking-widest text-zinc-400">
        Top Events
      </h2>
      {data.length === 0 ? (
        <p className="py-4 text-center text-xs text-zinc-600">No data yet</p>
      ) : (
        <ResponsiveContainer width="100%" height={180}>
          <BarChart
            data={data}
            layout="vertical"
            margin={{ top: 0, right: 16, bottom: 0, left: 0 }}
          >
            <XAxis
              type="number"
              tick={{ fill: '#52525b', fontSize: 11 }}
              axisLine={false}
              tickLine={false}
            />
            <YAxis
              type="category"
              dataKey="event"
              width={130}
              tick={{ fill: '#a1a1aa', fontSize: 11, fontFamily: 'monospace' }}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip
              contentStyle={{
                background: '#18181b',
                border: '1px solid #3f3f46',
                borderRadius: '6px',
                fontSize: '12px',
                color: '#f4f4f5',
              }}
              cursor={{ fill: 'rgba(6,182,212,0.05)' }}
            />
            <Bar dataKey="count" radius={[0, 4, 4, 0]}>
              {data.map((_, i) => (
                <Cell key={i} fill={BAR_COLORS[i] ?? BAR_COLORS[BAR_COLORS.length - 1]} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}
