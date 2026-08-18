'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import {
  NewRunDialog,
  type RlRunPreset,
} from '@/components/training/rl-runs-panel'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardFooter, CardHeader } from '@/components/ui/card'

const TEMPLATES = [
  {
    name: 'Reverse Text',
    description: 'The smallest end-to-end RL profile: train Qwen3 0.6B on a deterministic single-turn task. Ideal for validating a SkyScale setup before a longer run.',
    tag: 'Qwen3 0.6B · A10G',
    badge: 'Single-turn quickstart',
    icon: '↩️',
    source: 'https://github.com/PrimeIntellect-ai/prime-rl/tree/main/examples/basic/reverse-text',
    preset: {
      base_model: 'Qwen/Qwen3-0.6B',
      num_workers: 2,
      gpu_model: 'a10g',
    },
  },
  {
    name: 'Wordle',
    description: 'A multi-turn game profile for teaching Qwen3 1.7B to improve across stateful episodes and delayed rewards.',
    tag: 'Qwen3 1.7B · H100',
    badge: 'Multi-turn RL',
    icon: '🟩',
    source: 'https://github.com/PrimeIntellect-ai/prime-rl/tree/main/examples/basic/wordle',
    preset: {
      base_model: 'Qwen/Qwen3-1.7B',
      num_workers: 4,
      gpu_model: 'h100',
    },
  },
  {
    name: 'Alphabet Sort',
    description: 'A compact multi-turn reasoning profile based on Prime Intellect’s LoRA RL example, sized for fast iteration with Qwen3 4B.',
    tag: 'Qwen3 4B · H100',
    badge: 'Multi-turn reasoning',
    icon: '🔤',
    source: 'https://github.com/PrimeIntellect-ai/prime-rl/tree/main/examples/basic/alphabet-sort',
    preset: {
      base_model: 'Qwen/Qwen3-4B-Instruct-2507',
      num_workers: 4,
      gpu_model: 'h100',
    },
  },
  {
    name: 'Wiki Search',
    description: 'A tool-use profile for training Qwen3 4B to search, inspect retrieved context, and answer across longer multi-turn trajectories.',
    tag: 'Qwen3 4B · H100',
    badge: 'Tool use',
    icon: '🔎',
    source: 'https://github.com/PrimeIntellect-ai/prime-rl/tree/main/examples/basic/wiki-search',
    preset: {
      base_model: 'Qwen/Qwen3-4B-Instruct-2507',
      num_workers: 8,
      gpu_model: 'h100',
    },
  },
  {
    name: 'Hendrycks Sanity',
    description: 'A math-reasoning sanity-check profile using examples the base model solves inconsistently, useful for validating reward and algorithm changes.',
    tag: 'DeepSeek 1.5B · A100',
    badge: 'Math reasoning',
    icon: '∑',
    source: 'https://github.com/PrimeIntellect-ai/prime-rl/tree/main/examples/basic/hendrycks-sanity',
    preset: {
      base_model: 'deepseek-ai/DeepSeek-R1-Distill-Qwen-1.5B',
      num_workers: 4,
      gpu_model: 'a100',
    },
  },
] satisfies Array<{
  name: string
  description: string
  tag: string
  badge: string
  icon: string
  source: string
  preset: RlRunPreset
}>

export default function TemplatesPage() {
  const router = useRouter()
  const [selectedTemplate, setSelectedTemplate] = useState<(typeof TEMPLATES)[number] | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)

  function launch(tpl: typeof TEMPLATES[0]) {
    setSelectedTemplate(tpl)
    setDialogOpen(true)
  }

  return (
    <DashboardShell connected={false}>
      <PageHeader
        title="Templates"
        description="Prime Intellect–inspired RL profiles adapted to SkyScale’s model, GPU, and rollout controls."
      />

      <div className="flex-1 overflow-y-auto p-4 sm:p-6">
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {TEMPLATES.map(tpl => (
            <Card
              key={tpl.name}
              className="group cursor-pointer transition-colors hover:border-primary/40 hover:bg-accent/20"
              onClick={() => launch(tpl)}
            >
              <CardHeader className="space-y-3 pb-3">
                <div className="flex items-start gap-3">
                  <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-muted/50 text-lg">
                    {tpl.icon}
                  </div>
                  <div className="min-w-0 space-y-1.5">
                    <h3 className="text-sm font-semibold tracking-tight">{tpl.name}</h3>
                    <Badge variant="outline" className="text-[10px] font-medium">
                      {tpl.tag}
                    </Badge>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="pb-3">
                <p className="text-sm leading-relaxed text-muted-foreground">{tpl.description}</p>
              </CardContent>
              <CardFooter className="flex items-center justify-between border-t border-border pt-3">
                <a
                  href={tpl.source}
                  target="_blank"
                  rel="noreferrer"
                  className="text-xs text-muted-foreground hover:text-foreground"
                  onClick={e => e.stopPropagation()}
                >
                  {tpl.badge}
                </a>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={e => {
                    e.stopPropagation()
                    launch(tpl)
                  }}
                >
                  Configure run
                </Button>
              </CardFooter>
            </Card>
          ))}
        </div>
      </div>

      <NewRunDialog
        key={selectedTemplate?.name ?? 'default'}
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        onCreated={() => {
          setDialogOpen(false)
          router.push('/lab')
        }}
        preset={selectedTemplate?.preset}
      />
    </DashboardShell>
  )
}
