'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
  BarChart3,
  Box,
  Cpu,
  LayoutGrid,
  TrendingUp,
} from 'lucide-react'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { AccountSettings } from '@/components/dashboard/account-settings'
import { LogoMark } from '@/components/marketing/logo-mark'
import { authClient } from '@/lib/auth-client'
import { cn } from '@/lib/utils'

const NAV = [
  {
    section: 'Lab',
    items: [
      { label: 'Training', href: '/lab', icon: TrendingUp },
      { label: 'Templates', href: '/templates', icon: LayoutGrid },
      { label: 'Sandboxes', href: '/faas', icon: Box },
    ],
  },
  {
    section: 'Compute',
    items: [
      { label: 'On-Demand GPUs', href: '/gpus', icon: Cpu },
      { label: 'Load Speed', href: '/benchmarks', icon: BarChart3 },
    ],
  },
]

function profileInitials(name?: string | null, email?: string | null) {
  if (name) {
    const parts = name.trim().split(/\s+/).filter(Boolean)
    if (parts.length >= 2) return `${parts[0][0]}${parts[1][0]}`.toUpperCase()
    return parts[0]?.slice(0, 2).toUpperCase() ?? '?'
  }
  if (email) return email.slice(0, 2).toUpperCase()
  return '?'
}

export function Sidebar({ connected }: { connected: boolean }) {
  const pathname = usePathname()
  const { data: session, isPending } = authClient.useSession()
  const displayName = session?.user.name ?? session?.user.email ?? 'Guest'
  const subtitle = session ? 'Personal' : 'Sign in for account settings'

  return (
    <aside className="flex h-full w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
      <div className="flex items-center px-4 py-4">
        <Link href="/" className="transition-opacity hover:opacity-90">
          <LogoMark size={40} />
        </Link>
      </div>

      <Separator className="bg-sidebar-border" />

      <ScrollArea className="flex-1 px-2 py-3">
        <nav className="space-y-4">
          {NAV.map(group => (
            <div key={group.section}>
              <p className="mb-1 px-2 text-[10px] font-semibold uppercase tracking-widest text-sidebar-foreground/70">
                {group.section}
              </p>
              <div className="space-y-0.5">
                {group.items.map(item => {
                  const active = pathname === item.href
                  const Icon = item.icon
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      className={cn(
                        'flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm transition-all',
                        active
                          ? 'bg-accent font-medium text-sidebar-accent-foreground'
                          : 'text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
                      )}
                    >
                      <Icon
                        className={cn('size-4 shrink-0', active ? 'text-foreground' : 'opacity-70')}
                      />
                      <span className="truncate">{item.label}</span>
                    </Link>
                  )
                })}
              </div>
            </div>
          ))}
        </nav>
      </ScrollArea>

      <Separator className="bg-sidebar-border" />

      <Sheet>
        <SheetTrigger asChild>
          <button
            type="button"
            className="flex w-full items-center gap-2.5 px-3 py-3 text-left transition-colors hover:bg-sidebar-accent/60"
          >
            <Avatar className="size-8">
              <AvatarFallback className="bg-muted text-xs font-medium text-foreground">
                {isPending ? '…' : profileInitials(session?.user.name, session?.user.email)}
              </AvatarFallback>
            </Avatar>
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-medium text-sidebar-accent-foreground">
                {isPending ? 'Loading…' : displayName}
              </p>
              <p className="truncate text-[11px] text-sidebar-foreground">{subtitle}</p>
            </div>
            <span
              className={cn(
                'size-2 shrink-0 rounded-full',
                connected ? 'bg-[var(--success)]' : 'bg-muted-foreground/40',
              )}
              title={connected ? 'Connected' : 'Disconnected'}
            />
          </button>
        </SheetTrigger>
        <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-md">
          <SheetHeader>
            <SheetTitle>Account settings</SheetTitle>
            <SheetDescription>Manage your account and workspace.</SheetDescription>
          </SheetHeader>
          <div className="mt-6">
            <AccountSettings />
          </div>
        </SheetContent>
      </Sheet>
    </aside>
  )
}
