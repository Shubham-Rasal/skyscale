import { cn } from '@/lib/utils'

interface StatBadgeProps {
  label: string
  value: number | string
  tone?: 'primary' | 'success' | 'warning' | 'running' | 'muted'
  className?: string
}

const toneStyles = {
  primary: 'border-border bg-card',
  success: 'border-border bg-card',
  warning: 'border-border bg-card',
  running: 'border-border bg-card',
  muted: 'border-border bg-card',
}

const valueStyles = {
  primary: 'text-foreground',
  success: 'text-[var(--success)]',
  warning: 'text-[var(--warning)]',
  running: 'text-foreground',
  muted: 'text-muted-foreground',
}

export function StatBadge({ label, value, tone = 'primary', className }: StatBadgeProps) {
  return (
    <div
      className={cn(
        'flex items-center gap-1.5 rounded-sm border px-2.5 py-1.5 text-xs',
        toneStyles[tone],
        className,
      )}
    >
      <span className={cn('font-semibold tabular-nums', valueStyles[tone])}>{value}</span>
      <span className="text-muted-foreground">{label}</span>
    </div>
  )
}
