'use client'

import { Sidebar } from '@/components/Sidebar'
import { LogoMark } from '@/components/marketing/logo-mark'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'
import { Menu } from 'lucide-react'

interface DashboardShellProps {
  connected?: boolean
  children: React.ReactNode
  className?: string
}

export function DashboardShell({ connected = false, children, className }: DashboardShellProps) {
  return (
    <div className={cn('relative flex h-dvh overflow-hidden bg-background', className)}>
      <div className="pointer-events-none absolute inset-0 bg-galaxy-subtle opacity-60" />
      <Sidebar connected={connected} className="relative hidden lg:flex" />
      <div className="relative flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="flex h-14 shrink-0 items-center justify-between border-b border-border bg-background/90 px-4 backdrop-blur-md lg:hidden">
          <LogoMark size={32} />
          <Sheet>
            <SheetTrigger asChild>
              <Button variant="outline" size="icon" aria-label="Open navigation">
                <Menu className="size-4" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="w-[min(18rem,88vw)] p-0">
              <SheetTitle className="sr-only">Navigation</SheetTitle>
              <Sidebar connected={connected} className="w-full border-r-0" />
            </SheetContent>
          </Sheet>
        </div>
        {children}
      </div>
    </div>
  )
}
