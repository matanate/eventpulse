const PALETTE: Record<string, string> = {
  page_viewed:        'var(--color-chart-1)',
  user_signed_up:     'var(--color-chart-2)',
  checkout_completed: 'var(--color-chart-3)',
  button_clicked:     'var(--color-chart-4)',
  login_failed:       'var(--color-chart-5)',
}

const FALLBACK = [
  'var(--color-chart-1)',
  'var(--color-chart-2)',
  'var(--color-chart-3)',
  'var(--color-chart-4)',
  'var(--color-chart-5)',
]

export function eventColor(name: string, fallbackIndex = 0): string {
  return PALETTE[name] ?? FALLBACK[fallbackIndex % FALLBACK.length]!
}
