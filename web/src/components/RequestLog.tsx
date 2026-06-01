import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

export interface RequestEntry {
  id: number
  method: string
  path: string
  status: number
  latencyMs: number
}

interface RequestLogProps {
  entries: RequestEntry[]
}

function statusVariant(status: number): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (status >= 200 && status < 300) return 'default'
  if (status === 429) return 'secondary'
  return 'destructive'
}

export function RequestLog({ entries }: RequestLogProps) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
          Request Log
        </CardTitle>
      </CardHeader>
      <CardContent>
        {entries.length === 0 ? (
          <p className="py-2 text-center text-xs text-muted-foreground">
            Send events to see live requests
          </p>
        ) : (
          <div className="space-y-1.5 font-mono text-xs">
            {entries.map((entry) => (
              <div key={entry.id} className="flex items-center gap-2 min-w-0">
                <span
                  className={cn(
                    'shrink-0 text-[10px] font-bold uppercase',
                    entry.method === 'GET' ? 'text-muted-foreground/50' : 'text-primary/70',
                  )}
                >
                  {entry.method}
                </span>
                <span className="truncate flex-1 text-muted-foreground text-[11px]">
                  {entry.path}
                </span>
                <Badge
                  variant={statusVariant(entry.status)}
                  className="shrink-0 h-4 px-1.5 text-[10px]"
                >
                  {entry.status}
                </Badge>
                <span className="text-muted-foreground/50 shrink-0 tabular-nums text-[10px]">
                  {entry.latencyMs}ms
                </span>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
