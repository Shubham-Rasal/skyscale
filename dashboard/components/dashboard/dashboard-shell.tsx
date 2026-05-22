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
    <div className={cn('flex h-screen overflow-hidden bg-background', className)}>
      <Sidebar connected={connected} />
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {children}
      </div>
    </div>
  )
}
