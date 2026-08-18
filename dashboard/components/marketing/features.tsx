import { Box, Layers, TrendingUp, Zap } from 'lucide-react'

const SECTIONS = [
  {
    id: 'lab',
    fig: '01',
    icon: Layers,
    title: 'RL Environments',
    subtitle: 'Sandboxes are your step() function',
    description:
      'Every Firecracker microVM is a fully isolated RL environment. Reset spins up a fresh VM, step executes code against test cases, and the pass rate becomes the reward — no reward model required.',
    bullets: [
      'Gym-like reset / step / close API',
      'Ground-truth execution feedback at scale',
      'Fully isolated episodes per worker',
    ],
    code: `POST /api/rl/env/reset  →  { sandbox_id, prompt }
POST /api/rl/env/step   →  { reward, passed_tests }
POST /api/rl/env/close  →  204`,
  },
  {
    id: 'training',
    fig: '02',
    icon: TrendingUp,
    title: 'Hosted Training',
    subtitle: 'Distributed async GRPO at scale',
    description:
      'Cheap CPU workers collect trajectories asynchronously while a GPU trainer consumes batches independently. Scale workers and trainer without touching the rest of the pipeline.',
    bullets: [
      'Policy server on vLLM with weight hot-swap',
      'Experience buffer decouples rollouts from training',
      'Live metrics in the Lab dashboard',
    ],
    code: `POST /api/rl/runs
  { base_model, num_workers, gpu_model }

→ spawns policy server, workers, trainer`,
    reverse: true,
  },
  {
    id: 'sandboxes',
    fig: '03',
    icon: Box,
    title: 'Sandboxes',
    subtitle: 'Secure code execution for agents',
    description:
      'Deploy containerized functions and Railway-backed sandboxes for FaaS workloads. Run agent tools, eval harnesses, and custom environments with managed lifecycle.',
    bullets: [
      'Template-based deployment',
      'Container lifecycle management',
      'Invoke via REST API',
    ],
    code: `POST /api/faas/containers
  { template_id, env: { ... } }`,
  },
]

export function MarketingFeatures() {
  return (
    <section id="features" className="relative border-t border-border/60 py-16 sm:py-24">
      <div className="pointer-events-none absolute inset-0 bg-galaxy-subtle" />
      <div className="relative mx-auto max-w-6xl px-4 sm:px-6">
        <div className="mx-auto mb-12 max-w-2xl text-center sm:mb-16">
          <p className="text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">Platform</p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight sm:text-4xl">
            The open RL stack for post-training
          </h2>
          <p className="mt-4 text-muted-foreground">
            From isolated execution environments to managed GRPO training — everything you need to
            turn tasks into self-improving agents.
          </p>
        </div>

        <div className="space-y-16 sm:space-y-24">
          {SECTIONS.map(section => {
            const Icon = section.icon
            return (
              <article
                key={section.id}
                id={section.id}
                className="scroll-mt-24 grid min-w-0 items-center gap-8 lg:grid-cols-2 lg:gap-16"
              >
                <div className={section.reverse ? 'lg:order-2' : undefined}>
                  <div className="mb-4 flex items-center gap-3">
                    <span className="font-mono text-xs text-muted-foreground">FIG.{section.fig}</span>
                    <div className="flex size-9 items-center justify-center rounded-lg border border-border bg-background">
                      <Icon className="size-4 text-primary" />
                    </div>
                  </div>
                  <h3 className="text-xl font-semibold tracking-tight sm:text-2xl">{section.title}</h3>
                  <p className="mt-1 text-sm text-muted-foreground">{section.subtitle}</p>
                  <p className="mt-4 leading-relaxed text-muted-foreground">{section.description}</p>
                  <ul className="mt-6 space-y-2">
                    {section.bullets.map(bullet => (
                      <li key={bullet} className="flex items-start gap-2 text-sm text-foreground/85">
                        <Zap className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
                        {bullet}
                      </li>
                    ))}
                  </ul>
                </div>

                <div
                  className={`min-w-0 rounded-[var(--radius-card)] border border-border bg-card p-1 ${section.reverse ? 'lg:order-1' : ''}`}
                >
                  <div className="rounded-[8px] border border-border bg-background p-3 sm:p-5">
                    <div className="mb-3 flex items-center gap-2">
                      <span className="size-2 rounded-full bg-muted-foreground/40" />
                      <span className="size-2 rounded-full bg-muted-foreground/30" />
                      <span className="size-2 rounded-full bg-muted-foreground/20" />
                      <span className="ml-2 font-mono text-[10px] text-muted-foreground">
                        skyscale-api
                      </span>
                    </div>
                    <pre className="w-full max-w-full overflow-x-auto font-mono text-xs leading-relaxed text-foreground/90 sm:text-sm">
                      <code>{section.code}</code>
                    </pre>
                  </div>
                </div>
              </article>
            )
          })}
        </div>
      </div>
    </section>
  )
}
