'use client'

import Link from 'next/link'
import { useEffect, useState } from 'react'
import { ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SkyscaleLogo } from '@/components/marketing/logo'
import { cn } from '@/lib/utils'

const LINKS = [
  { label: 'Lab', href: '/#lab', num: '01' },
  { label: 'Training', href: '/#training', num: '02' },
  { label: 'Compute', href: '/#compute', num: '03' },
  { label: 'Sandboxes', href: '/#sandboxes', num: '04' },
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
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-4 px-6">
        <SkyscaleLogo showLabel={false} size={40} />

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
          <Button size="sm" asChild className="rounded-sm font-medium uppercase tracking-wide">
            <Link href="/lab">
              Start training
              <ArrowRight className="size-3.5" />
            </Link>
          </Button>
        </div>
      </div>
    </header>
  )
}
