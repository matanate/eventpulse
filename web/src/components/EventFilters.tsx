const EVENT_TYPES = [
  'page_viewed',
  'user_signed_up',
  'checkout_completed',
  'button_clicked',
  'login_failed',
] as const

interface EventFiltersProps {
  eventFilter: string
  userIdFilter: string
  onEventFilterChange: (v: string) => void
  onUserIdFilterChange: (v: string) => void
}

export function EventFilters({
  eventFilter,
  userIdFilter,
  onEventFilterChange,
  onUserIdFilterChange,
}: EventFiltersProps) {
  const hasFilters = Boolean(eventFilter || userIdFilter)

  return (
    <div className="flex flex-wrap items-center gap-2">
      <select
        value={eventFilter}
        onChange={(e) => onEventFilterChange(e.target.value)}
        aria-label="Filter by event type"
        className="rounded-md border border-zinc-700 bg-zinc-800 px-2 py-1 text-xs text-zinc-300 focus:border-cyan-500 focus:outline-none"
      >
        <option value="">All events</option>
        {EVENT_TYPES.map((t) => (
          <option key={t} value={t}>
            {t}
          </option>
        ))}
      </select>

      <input
        type="text"
        value={userIdFilter}
        onChange={(e) => onUserIdFilterChange(e.target.value)}
        placeholder="Filter by user_id"
        aria-label="Filter by user ID"
        className="min-w-[130px] flex-1 rounded-md border border-zinc-700 bg-zinc-800 px-2 py-1 text-xs text-zinc-300 placeholder-zinc-600 focus:border-cyan-500 focus:outline-none"
      />

      {hasFilters && (
        <button
          type="button"
          onClick={() => {
            onEventFilterChange('')
            onUserIdFilterChange('')
          }}
          className="text-xs text-zinc-500 transition-colors hover:text-zinc-300"
        >
          Clear
        </button>
      )}
    </div>
  )
}
