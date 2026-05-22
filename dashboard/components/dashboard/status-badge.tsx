import { cn } from '@/lib/utils'

const statusConfig: Record<string, { className: string; pulse?: boolean }> = {
  running: {
    className: 'border-[var(--running)]/25 bg-[var(--running-dim)] text-[var(--running)]',
    pulse: true,
  },
  starting: {
    className: 'border-[var(--warning)]/25 bg-[var(--warning)]/10 text-[var(--warning)]',
    pulse: true,
  },
  deploying: {
    className: 'border-[var(--running)]/25 bg-[var(--running-dim)] text-[var(--running)]',
    pulse: true,
  },
  creating: {
    className: 'border-[var(--running)]/25 bg-[var(--running-dim)] text-[var(--running)]',
    pulse: true,
  },
  stopping: {
    className: 'border-[var(--warning)]/25 bg-[var(--warning)]/10 text-[var(--warning)]',
    pulse: true,
  },
  completed: {
    className: 'border-[var(--success)]/25 bg-[var(--success-dim)] text-[var(--success)]',
  },
  ready: {
    className: 'border-[var(--success)]/25 bg-[var(--success-dim)] text-[var(--success)]',
  },
  stopped: {
    className: 'border-border bg-muted/50 text-muted-foreground',
  },
  queued: {
    className: 'border-border bg-muted/50 text-muted-foreground',
  },
  failed: {
    className: 'border-destructive/25 bg-destructive/10 text-destructive',
  },
  error: {
    className: 'border-destructive/25 bg-destructive/10 text-destructive',
  },
}

interface StatusBadgeProps {
  status: string
  className?: string
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const config = statusConfig[status] ?? statusConfig.stopped

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-[11px] font-medium capitalize',
        config.className,
        className,
      )}
    >
      <span
        className={cn(
          'size-1.5 rounded-full bg-current',
          config.pulse && 'animate-[pulse_1.8s_ease-in-out_infinite]',
        )}
      />
      {status}
    </span>
  )
}
