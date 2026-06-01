import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'

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
        className="rounded-md border border-input bg-background px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
      >
        <option value="">All events</option>
        {EVENT_TYPES.map((t) => (
          <option key={t} value={t}>
            {t}
          </option>
        ))}
      </select>

      <Input
        type="text"
        value={userIdFilter}
        onChange={(e) => onUserIdFilterChange(e.target.value)}
        placeholder="Filter by user_id"
        aria-label="Filter by user ID"
        className="h-8 min-w-[130px] flex-1 text-xs"
      />

      {hasFilters && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => {
            onEventFilterChange('')
            onUserIdFilterChange('')
          }}
          className="h-8 text-xs text-muted-foreground"
        >
          Clear
        </Button>
      )}
    </div>
  )
}
