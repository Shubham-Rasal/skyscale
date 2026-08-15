'use client'

import { Sidebar } from '@/components/Sidebar'
import { cn } from '@/lib/utils'

interface DashboardShellProps {
  connected?: boolean
  children: React.ReactNode
  className?: string
}

export function DashboardShell({ connected = false, children, className }: DashboardShellProps) {
  return (
    <div className={cn('relative flex h-screen overflow-hidden bg-background', className)}>
      <div className="pointer-events-none absolute inset-0 bg-galaxy-subtle opacity-60" />
      <Sidebar connected={connected} />
      <div className="relative flex min-w-0 flex-1 flex-col overflow-hidden">
        {children}
      </div>
    </div>
  )
}
