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
    <header className={cn('shrink-0 border-b border-border bg-background', className)}>
      <div className="flex h-14 items-center gap-3 px-6">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <h1 className="truncate text-base font-semibold tracking-tight text-foreground">
              {title}
            </h1>
            {badge && (
              <Badge variant={badgeVariant} className="h-5 px-1.5 text-[10px] font-semibold uppercase tracking-wider">
                {badge}
              </Badge>
            )}
          </div>
          {description && (
            <p className="mt-0.5 truncate text-xs text-muted-foreground">{description}</p>
          )}
        </div>

        {stats && (
          <div className="hidden items-center gap-2 sm:flex">{stats}</div>
        )}

        {actions && (
          <div className="flex shrink-0 items-center gap-2">{actions}</div>
        )}
      </div>

      {toolbar && (
        <div className="border-t border-border px-6 py-2.5">{toolbar}</div>
      )}
    </header>
  )
}
