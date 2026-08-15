'use client'

import Link from 'next/link'
import { useEffect, useState } from 'react'
import { ArrowRight, Menu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SkyscaleLogo } from '@/components/marketing/logo'
import { Sheet, SheetClose, SheetContent, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

const LINKS = [
  { label: 'Lab', href: '/#lab', num: '01' },
  { label: 'Training', href: '/#training', num: '02' },
  { label: 'Sandboxes', href: '/#sandboxes', num: '03' },
]

export function MarketingNav() {
  const [scrolled, setScrolled] = useState(false)

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 12)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  return (
    <header
      className={cn(
        'fixed inset-x-0 top-0 z-50 transition-all duration-300',
        scrolled
          ? 'border-b border-border/60 bg-background/80 backdrop-blur-md'
          : 'bg-transparent',
      )}
    >
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-3 px-4 sm:px-6">
        <SkyscaleLogo showLabel={false} size={36} />

        <nav className="hidden flex-1 items-center justify-center lg:flex">
          <div className="flex items-center divide-x divide-border/60 rounded-sm border border-border/40 bg-background/40 backdrop-blur-sm">
            {LINKS.map(link => (
              <Link
                key={link.href}
                href={link.href}
                className="group flex items-center gap-2 px-4 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground transition-colors hover:text-foreground"
              >
                <span className="font-mono text-[10px] text-muted-foreground/60 group-hover:text-muted-foreground">
                  {link.num}
                </span>
                {link.label}
              </Link>
            ))}
          </div>
        </nav>

        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" size="sm" asChild className="hidden text-muted-foreground sm:inline-flex">
            <Link href="/login">Login</Link>
          </Button>
          <Button size="sm" asChild className="hidden font-medium sm:inline-flex">
            <Link href="/lab">
              See live training
              <ArrowRight className="size-3.5" />
            </Link>
          </Button>
          <Sheet>
            <SheetTrigger asChild>
              <Button variant="outline" size="icon" className="lg:hidden" aria-label="Open menu">
                <Menu className="size-4" />
              </Button>
            </SheetTrigger>
            <SheetContent side="right" className="w-[min(20rem,88vw)]">
              <SheetTitle>Navigation</SheetTitle>
              <nav className="mt-6 flex flex-col gap-1">
                {LINKS.map(link => (
                  <SheetClose key={link.href} asChild>
                    <Link
                      href={link.href}
                      className="flex items-center gap-3 rounded-[var(--radius-control)] px-3 py-2.5 text-sm font-medium text-muted-foreground hover:bg-accent hover:text-foreground"
                    >
                      <span className="font-mono text-[10px] text-muted-foreground/70">{link.num}</span>
                      {link.label}
                    </Link>
                  </SheetClose>
                ))}
              </nav>
              <div className="mt-6 grid gap-2 border-t border-border pt-4">
                <Button asChild>
                  <Link href="/lab">See live training</Link>
                </Button>
                <Button variant="outline" asChild>
                  <Link href="/login">Login</Link>
                </Button>
              </div>
            </SheetContent>
          </Sheet>
        </div>
      </div>
    </header>
  )
}
