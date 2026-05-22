import { cn } from '@/lib/utils'

interface StatBadgeProps {
  label: string
  value: number | string
  tone?: 'primary' | 'success' | 'warning' | 'running' | 'muted'
  className?: string
}

const toneStyles = {
  primary: 'text-primary',
  success: 'text-[var(--success)]',
  warning: 'text-[var(--warning)]',
  running: 'text-[var(--running)]',
  muted: 'text-muted-foreground',
}

export function StatBadge({ label, value, tone = 'primary', className }: StatBadgeProps) {
  return (
    <div
      className={cn(
        'flex items-center gap-1.5 rounded-md border border-border bg-card px-2.5 py-1 text-xs',
        className,
      )}
    >
      <span className={cn('font-semibold tabular-nums', toneStyles[tone])}>{value}</span>
      <span className="text-muted-foreground">{label}</span>
    </div>
  )
}
