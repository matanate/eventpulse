import { useState, useCallback } from 'react'
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

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[10px] font-mono uppercase tracking-widest text-muted-foreground/40">
      {children}
    </p>
  )
}

export default function App() {
  const [requestLog, setRequestLog] = useState<RequestEntry[]>([])
  const [isPipelining, setIsPipelining] = useState(false)

  const logRequest = useCallback((entry: Omit<RequestEntry, 'id'>) => {
    setRequestLog((prev) => [
      { ...entry, id: Date.now() },
      ...prev.slice(0, 9),
    ])
  }, [])

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border px-6 py-4">
        <div className="mx-auto flex max-w-6xl items-center gap-3">
          <span className="h-2 w-2 rounded-full bg-primary animate-pulse" aria-hidden />
          <span className="font-mono text-sm font-semibold tracking-wide">EventPulse</span>
          <span className="text-muted-foreground/30">/</span>
          <span className="text-sm text-muted-foreground">live demo</span>
          <a
            href="/docs"
            target="_blank"
            rel="noopener noreferrer"
            className="ml-auto text-xs text-muted-foreground/50 transition-colors hover:text-muted-foreground"
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
      </header>

      <ConnectionBanner />

      <main className="mx-auto max-w-6xl px-6 py-8">
        <div className="mb-6">
          <h1 className="text-2xl font-bold tracking-tight">Event Analytics Pipeline</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Send events through the live API — auth, rate limiting, Redis Streams, and Postgres in
            action.
          </p>
        </div>

        <div className="mb-4">
          <PipelineCard isActive={isPipelining} />
        </div>

        <div className="mb-6">
          <QueueHealthCard />
        </div>

        <ErrorBoundary>
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-5">
            {/* Left column — ingestion controls */}
            <div className="lg:col-span-2 space-y-2">
              <SectionLabel>Ingestion Controls</SectionLabel>
              <div className="space-y-5">
                <EventSender onRequest={logRequest} onSendingChange={setIsPipelining} />
                <RequestLog entries={requestLog} />
              </div>
            </div>

            {/* Right column — analytics */}
            <div className="lg:col-span-3 space-y-2">
              <SectionLabel>Live Analytics</SectionLabel>
              <div className="space-y-5">
                <StatsPanel />
                <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
                  <ActivityTimeline />
                  <EventDistribution />
                </div>
                <TopEventsChart />
                <FunnelChart />
                <UserActivity />
                <EventFeed />
              </div>
            </div>
          </div>
        </ErrorBoundary>
      </main>

      <footer className="mt-8 border-t border-border px-6 py-4">
        <p className="mx-auto max-w-6xl text-center text-xs text-muted-foreground/40">
          Go · Chi · Redis Streams · PostgreSQL · pgx/v5 · Railway · Cloudflare Pages
        </p>
      </footer>
    </div>
  )
}
