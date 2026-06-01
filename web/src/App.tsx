import { ErrorBoundary } from './components/ErrorBoundary'
import { ConnectionBanner } from './components/ConnectionBanner'
import { EventSender } from './components/EventSender'
import { StatsPanel } from './components/StatsPanel'
import { EventFeed } from './components/EventFeed'
import { TopEventsChart } from './components/TopEventsChart'

export default function App() {
  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <header className="border-b border-zinc-800 px-6 py-4">
        <div className="mx-auto flex max-w-5xl items-center gap-3">
          <span className="h-2 w-2 rounded-full bg-cyan-400 animate-pulse" aria-hidden />
          <span className="font-mono text-sm font-semibold tracking-wide text-zinc-300">
            EventPulse
          </span>
          <span className="text-zinc-700">/</span>
          <span className="text-sm text-zinc-500">live demo</span>
          <a
            href="https://github.com/matanate/eventpulse"
            target="_blank"
            rel="noopener noreferrer"
            className="ml-auto text-xs text-zinc-600 transition-colors hover:text-zinc-400"
          >
            GitHub →
          </a>
        </div>
      </header>

      <ConnectionBanner />

      <main className="mx-auto max-w-5xl px-6 py-8">
        <div className="mb-8">
          <h1 className="text-2xl font-bold tracking-tight">Event Analytics</h1>
          <p className="mt-1 text-sm text-zinc-500">
            Send events through the live API and watch them propagate — auth, rate limiting,
            Redis Streams, and Postgres persistence in action.
          </p>
        </div>

        <ErrorBoundary>
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-5">
            <div className="lg:col-span-2">
              <EventSender />
            </div>

            <div className="lg:col-span-3 space-y-5">
              <StatsPanel />
              <div className="rounded-xl border border-zinc-800 bg-zinc-900 p-5">
                <TopEventsChart />
              </div>
              <div className="rounded-xl border border-zinc-800 bg-zinc-900 p-5">
                <EventFeed />
              </div>
            </div>
          </div>
        </ErrorBoundary>
      </main>

      <footer className="mt-8 border-t border-zinc-800 px-6 py-4">
        <p className="mx-auto max-w-5xl text-center text-xs text-zinc-700">
          Go · Redis Streams · PostgreSQL · Railway · Cloudflare Pages
        </p>
      </footer>
    </div>
  )
}
