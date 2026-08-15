import Image from 'next/image'
import Link from 'next/link'
import { ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'

export function MarketingHero() {
  return (
    <section className="relative min-h-[92vh] overflow-hidden">
      <div className="absolute inset-0">
        <Image
          src="/hero.png"
          alt="Deep space galaxy visualization"
          fill
          priority
          className="object-cover object-[center_40%]"
          sizes="100vw"
        />
        <div className="absolute inset-0 bg-gradient-to-r from-background via-background/92 to-background/20" />
        <div className="absolute inset-0 bg-gradient-to-t from-background via-transparent to-background/40" />
        <div className="pointer-events-none absolute inset-0 bg-galaxy-radial opacity-50" />
      </div>

      <div className="relative mx-auto flex min-h-[92vh] max-w-6xl flex-col justify-center px-6 pb-20 pt-28 md:pt-32">
        <p className="mb-5 text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
          The Open RL Stack
        </p>

        <h1 className="max-w-2xl text-4xl font-semibold leading-[1.05] tracking-tight sm:text-5xl lg:text-[3.25rem]">
          Own Your Intelligence
        </h1>

        <p className="mt-5 max-w-lg text-base leading-relaxed text-muted-foreground sm:text-[1.05rem]">
          Post-train any LLM with distributed async GRPO — rollout workers, policy servers, and
          trainers orchestrated from one API call.
        </p>

        <div className="mt-8 flex flex-wrap items-center gap-3">
          <Button size="lg" asChild className="h-11 rounded-sm px-6 font-medium">
            <Link href="/lab">
              Start training
              <ArrowRight className="size-4" />
            </Link>
          </Button>
          <Button
            size="lg"
            variant="outline"
            asChild
            className="h-11 rounded-sm border-border/60 bg-background/20 px-6 backdrop-blur-sm hover:bg-accent/50"
          >
            <Link href="/#lab">Explore the stack</Link>
          </Button>
        </div>

        <p className="mt-8 font-mono text-sm text-terminal">
          $ curl -X POST $SKYSCALE/api/rl/runs
        </p>

        <div className="mt-14 grid max-w-lg grid-cols-3 gap-px border border-border/60 bg-border/60">
          {[
            { label: 'Workers', value: 'N × CPU' },
            { label: 'Policy', value: 'vLLM GPU' },
            { label: 'Trainer', value: 'GRPO' },
          ].map(item => (
            <div key={item.label} className="bg-background/80 px-3 py-3 backdrop-blur-sm">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground">
                {item.label}
              </p>
              <p className="mt-1 font-mono text-xs text-foreground">{item.value}</p>
            </div>
          ))}
        </div>

        <p className="mt-16 text-[11px] uppercase tracking-[0.15em] text-muted-foreground/70">
          Built for teams training at scale
        </p>
      </div>
    </section>
  )
}
