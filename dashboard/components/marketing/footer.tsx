import Link from 'next/link'
import { SkyscaleLogo } from '@/components/marketing/logo'

export function MarketingFooter() {
  return (
    <footer className="relative border-t border-border/60 bg-background py-16">
      <div className="pointer-events-none absolute inset-0 bg-galaxy-subtle opacity-30" />
      <div className="relative mx-auto flex max-w-6xl flex-col gap-10 px-6 md:flex-row md:items-start md:justify-between">
        <div>
          <SkyscaleLogo subtitle="Reinforcement Learning as a Service" />
          <p className="mt-4 max-w-xs text-sm text-muted-foreground">
            Post-train any LLM on any task with distributed async RL — without managing clusters.
          </p>
        </div>

        <div className="grid grid-cols-2 gap-10 sm:grid-cols-3">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wider text-foreground">Platform</p>
            <ul className="mt-3 space-y-2 text-sm text-muted-foreground">
              <li><Link href="/lab" className="hover:text-foreground">Lab</Link></li>
              <li><Link href="/templates" className="hover:text-foreground">Templates</Link></li>
              <li><Link href="/gpus" className="hover:text-foreground">Compute</Link></li>
            </ul>
          </div>
          <div>
            <p className="text-xs font-semibold uppercase tracking-wider text-foreground">Developers</p>
            <ul className="mt-3 space-y-2 text-sm text-muted-foreground">
              <li><a href="https://github.com" className="hover:text-foreground">GitHub</a></li>
              <li><Link href="/login" className="hover:text-foreground">Login</Link></li>
            </ul>
          </div>
          <div>
            <p className="text-xs font-semibold uppercase tracking-wider text-foreground">Product</p>
            <ul className="mt-3 space-y-2 text-sm text-muted-foreground">
              <li><Link href="/faas" className="hover:text-foreground">Sandboxes</Link></li>
              <li><Link href="/benchmarks" className="hover:text-foreground">Benchmarks</Link></li>
            </ul>
          </div>
        </div>
      </div>

      <div className="relative mx-auto mt-12 max-w-6xl border-t border-border/60 px-6 pt-8">
        <p className="text-xs text-muted-foreground">© {new Date().getFullYear()} Skyscale</p>
      </div>
    </footer>
  )
}
