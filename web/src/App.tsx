import { useState, useCallback, useEffect, useRef } from 'react'
import { ErrorBoundary } from './components/ErrorBoundary'
import { ConnectionBanner } from './components/ConnectionBanner'
import { EventSender } from './components/EventSender'
import { StatsPanel } from './components/StatsPanel'
import { EventFeed } from './components/EventFeed'
import { TopEventsChart } from './components/TopEventsChart'
import { EventDistribution } from './components/EventDistribution'
import { ActivityTimeline } from './components/ActivityTimeline'
import { PipelineCard } from './components/PipelineCard'
import { QueueHealthCard } from './components/QueueHealthCard'
import { RequestLog, type RequestEntry } from './components/RequestLog'
import { UserActivity } from './components/UserActivity'
import { FunnelChart } from './components/FunnelChart'
import { RetentionGrid } from './components/RetentionGrid'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './components/ui/tabs'
import { getConnectionStatus, subscribeToConnection } from './lib/connection'
import { cn } from './lib/utils'

function useOnlineStatus() {
  const [online, setOnline] = useState(getConnectionStatus())
  useEffect(() => subscribeToConnection(() => setOnline(getConnectionStatus())), [])
  return online
}

export default function App() {
  const [requestLog, setRequestLog] = useState<RequestEntry[]>([])
  const [isPipelining, setIsPipelining] = useState(false)
  const online = useOnlineStatus()
  const nextIdRef = useRef(0)

  const logRequest = useCallback((entry: Omit<RequestEntry, 'id'>) => {
    setRequestLog((prev) => [{ ...entry, id: ++nextIdRef.current }, ...prev.slice(0, 9)])
  }, [])

  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* ── Header ── */}
      <header className="sticky top-0 z-10 border-b border-border bg-background/95 px-6 py-3 backdrop-blur">
        <div className="mx-auto flex max-w-7xl items-center gap-3">
          <span className="h-2 w-2 rounded-full bg-primary animate-pulse" aria-hidden />
          <span className="font-mono text-sm font-semibold tracking-wide">EventPulse</span>
          <span className="text-muted-foreground/30">/</span>
          <span className="text-sm text-muted-foreground">live demo</span>

          {/* API connection status */}
          <div className="ml-3 flex items-center gap-1.5">
            <span
              className={cn(
                'h-1.5 w-1.5 rounded-full transition-colors',
                online ? 'bg-emerald-500' : 'bg-red-500',
              )}
            />
            <span className="text-xs text-muted-foreground/60">
              {online ? 'API connected' : 'API offline'}
            </span>
          </div>

          <div className="ml-auto flex items-center gap-5">
            <a
              href="/docs"
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-muted-foreground/50 transition-colors hover:text-muted-foreground"
            >
              API Docs →
            </a>
            <a
              href="https://github.com/matanate/eventpulse"
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-muted-foreground/50 transition-colors hover:text-muted-foreground"
            >
              GitHub →
            </a>
          </div>
        </div>
      </header>

      <ConnectionBanner />

      <main className="mx-auto max-w-7xl px-6 py-8">
        {/* ── Page intro ── */}
        <div className="mb-6">
          <h1 className="text-2xl font-bold tracking-tight">Event Analytics Pipeline</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            A production-style Go backend — auth, Redis Streams, PostgreSQL, and real-time SSE.
            Send events below and watch the pipeline work live.
          </p>
        </div>

        {/* ── Main tab navigation ── */}
        <ErrorBoundary>
          <Tabs defaultValue="monitor">
            <TabsList className="mb-6 h-auto w-full justify-start gap-0 rounded-none border-b border-border bg-transparent p-0">
              <TabsTrigger
                value="monitor"
                className="rounded-none border-b-2 border-transparent px-5 py-2.5 text-sm font-medium transition-none data-[state=active]:border-primary data-[state=active]:text-primary data-[state=active]:shadow-none"
              >
                Send &amp; Monitor
              </TabsTrigger>
              <TabsTrigger
                value="analytics"
                className="rounded-none border-b-2 border-transparent px-5 py-2.5 text-sm font-medium transition-none data-[state=active]:border-primary data-[state=active]:text-primary data-[state=active]:shadow-none"
              >
                Analytics
              </TabsTrigger>
              <TabsTrigger
                value="infrastructure"
                className="rounded-none border-b-2 border-transparent px-5 py-2.5 text-sm font-medium transition-none data-[state=active]:border-primary data-[state=active]:text-primary data-[state=active]:shadow-none"
              >
                Infrastructure
              </TabsTrigger>
            </TabsList>

            {/* ══ Tab 1: Send & Monitor ══ */}
            <TabsContent value="monitor" className="mt-0">
              <div className="mb-5">
                <StatsPanel />
              </div>

              <div className="grid grid-cols-1 gap-6 lg:grid-cols-5">
                {/* Left — event sender */}
                <div className="space-y-4 lg:col-span-2">
                  <EventSender onRequest={logRequest} onSendingChange={setIsPipelining} />
                  <RequestLog entries={requestLog} />
                </div>

                {/* Right — live feed */}
                <div className="lg:col-span-3">
                  <EventFeed />
                </div>
              </div>
            </TabsContent>

            {/* ══ Tab 2: Analytics ══ */}
            <TabsContent value="analytics" className="mt-0">
              <div className="space-y-5">
                <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
                  <ActivityTimeline />
                  <EventDistribution />
                </div>
                <TopEventsChart />
                <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
                  <FunnelChart />
                  <RetentionGrid />
                </div>
                <UserActivity />
              </div>
            </TabsContent>

            {/* ══ Tab 3: Infrastructure ══ */}
            <TabsContent value="infrastructure" className="mt-0">
              <div className="space-y-5">
                <PipelineCard isActive={isPipelining} />
                <QueueHealthCard />

                {/* Tech stack callout */}
                <div className="rounded-xl border border-border bg-card p-5">
                  <h3 className="mb-3 text-xs font-mono uppercase tracking-widest text-muted-foreground">
                    Stack
                  </h3>
                  <div className="grid grid-cols-2 gap-x-8 gap-y-2 text-xs sm:grid-cols-3">
                    {[
                      ['Language', 'Go 1.22+'],
                      ['Router', 'Chi v5'],
                      ['Database', 'PostgreSQL 16 · pgx/v5'],
                      ['Queue', 'Redis 7 Streams'],
                      ['Auth', 'Bearer token · SHA-256'],
                      ['Rate limit', 'Redis sliding window'],
                      ['Real-time', 'Server-Sent Events (SSE)'],
                      ['Webhooks', 'HMAC-SHA256 signed'],
                      ['Metrics', 'Prometheus + Grafana'],
                      ['Migrations', 'golang-migrate'],
                      ['Deploy', 'Railway'],
                      ['Frontend', 'React + Vite · Cloudflare Pages'],
                    ].map(([label, value]) => (
                      <div key={label} className="flex items-baseline gap-1.5">
                        <span className="shrink-0 text-muted-foreground/50">{label}</span>
                        <span className="font-mono text-foreground/80">{value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </TabsContent>
          </Tabs>
        </ErrorBoundary>
      </main>

      {/* ── Footer ── */}
      <footer className="mt-12 border-t border-border px-6 py-4">
        <p className="mx-auto max-w-7xl text-center text-xs text-muted-foreground/40">
          Go · Chi · Redis Streams · PostgreSQL · pgx/v5 · Railway · Cloudflare Pages
        </p>
      </footer>
    </div>
  )
}
