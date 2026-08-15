'use client'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

interface PageHeaderProps {
  title: string
  description?: string
  badge?: string
  badgeVariant?: 'default' | 'secondary' | 'outline' | 'destructive'
  actions?: React.ReactNode
  stats?: React.ReactNode
  toolbar?: React.ReactNode
  className?: string
}

export function PageHeader({
  title,
  description,
  badge,
  badgeVariant = 'secondary',
  actions,
  stats,
  toolbar,
  className,
}: PageHeaderProps) {
  return (
    <header className={cn('relative shrink-0 border-b border-border bg-background', className)}>
      <div className="pointer-events-none absolute inset-0 bg-galaxy-subtle opacity-30" />
      <div className="relative flex min-h-[4.25rem] flex-wrap items-center gap-3 px-4 py-3 sm:px-6">
        <div className="min-w-[10rem] flex-1">
          <div className="flex items-center gap-2.5">
            <h1 className="truncate text-sm font-semibold tracking-tight text-foreground sm:text-base">
              {title}
            </h1>
            {badge && (
              <Badge variant={badgeVariant} className="h-5 px-1.5 text-[10px] font-semibold uppercase tracking-wider">
                {badge}
              </Badge>
            )}
          </div>
          {description && (
            <p className="mt-0.5 line-clamp-2 text-[11px] text-muted-foreground sm:text-xs">{description}</p>
          )}
        </div>

        {stats && (
          <div className="hidden items-center gap-2 xl:flex">{stats}</div>
        )}

        {actions && (
          <div className="ml-auto flex shrink-0 items-center gap-2">{actions}</div>
        )}
      </div>

      {toolbar && (
        <div className="relative overflow-x-auto border-t border-border px-4 py-2.5 sm:px-6">{toolbar}</div>
      )}
    </header>
  )
}
