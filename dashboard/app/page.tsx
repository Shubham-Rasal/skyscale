import { MarketingNav } from '@/components/marketing/nav'
import { MarketingHero } from '@/components/marketing/hero'
import { MarketingFeatures } from '@/components/marketing/features'
import { MarketingFooter } from '@/components/marketing/footer'
import Link from 'next/link'
import { ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'

export default function HomePage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <MarketingNav />
      <main>
        <MarketingHero />
        <MarketingFeatures />

        <section className="relative border-t border-border/60 py-24">
          <div className="pointer-events-none absolute inset-0 bg-galaxy-subtle opacity-40" />
          <div className="relative mx-auto max-w-3xl px-6 text-center">
            <h2 className="text-3xl font-semibold tracking-tight sm:text-4xl">
              Be your own Lab.
            </h2>
            <p className="mt-4 text-muted-foreground">
              Turn production traces into the next training run. Start a distributed GRPO job in
              seconds and watch live metrics as your agent improves.
            </p>
            <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
              <Button size="lg" asChild>
                <Link href="/lab">
                  Open Lab
                  <ArrowRight className="size-4" />
                </Link>
              </Button>
              <Button size="lg" variant="outline" asChild>
                <Link href="/login">Sign in</Link>
              </Button>
            </div>
          </div>
        </section>
      </main>
      <MarketingFooter />
    </div>
  )
}
