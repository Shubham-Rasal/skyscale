'use client'

import { useState } from 'react'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { SubmitJobDialog } from '@/components/SubmitJobDialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardFooter, CardHeader } from '@/components/ui/card'

const TEMPLATES = [
  {
    name: 'MNIST CNN',
    dialogLabel: 'MNIST CNN',
    description: 'Trains a 3-layer CNN on MNIST using PyTorch + CUDA on an A100. Streams loss, accuracy, and GPU utilisation back to the dashboard every 50 batches.',
    tag: 'GPU · Akash',
    hardware: 'gpu',
    gpuModel: 'a100',
    jobType: 'training_run',
    badge: 'Image Classification',
    icon: '🧠',
  },
  {
    name: 'MNIST CNN on Hugging Face',
    dialogLabel: 'MNIST HF',
    description: 'Runs the same MNIST PyTorch trainer on Hugging Face Jobs using an A10G GPU. Useful for quick smoke tests and GPU sandbox runs without Akash setup.',
    tag: 'GPU · Hugging Face',
    hardware: 'gpu',
    gpuModel: 'a10g',
    jobType: 'training_run',
    badge: 'Image Classification',
    icon: '🤗',
  },
  {
    name: 'CartPole PPO',
    dialogLabel: 'CartPole GPU',
    description: 'Trains a PPO agent on CartPole-v1 with stable-baselines3. Streams live reward and loss curves back to the dashboard.',
    tag: 'GPU · Akash',
    hardware: 'gpu',
    gpuModel: 'rtx3090',
    jobType: 'training_run',
    badge: 'Reinforcement Learning',
    icon: '🎮',
  },
  {
    name: 'CPU FaaS Function',
    dialogLabel: 'CPU FaaS',
    description: 'Run any serverless function on a Firecracker microVM. Fast cold starts, isolated execution, automatic cleanup after completion.',
    tag: 'CPU · Firecracker',
    hardware: 'cpu',
    gpuModel: '',
    jobType: 'faas_function',
    badge: 'Serverless',
    icon: '⚡',
  },
  {
    name: 'RL Environment',
    dialogLabel: 'Custom',
    description: 'Host a gymnasium-compatible RL environment on Akash GPU compute, accepting remote agent connections over the network.',
    tag: 'GPU · Akash',
    hardware: 'gpu',
    gpuModel: 'rtx3090',
    jobType: 'rl_env',
    badge: 'Reinforcement Learning',
    icon: '🤖',
  },
]

export default function TemplatesPage() {
  const [selectedLabel, setSelectedLabel] = useState<string | undefined>(undefined)
  const [dialogOpen, setDialogOpen] = useState(false)

  function launch(tpl: typeof TEMPLATES[0]) {
    setSelectedLabel(tpl.dialogLabel)
    setDialogOpen(true)
  }

  return (
    <DashboardShell connected={false}>
      <PageHeader
        title="Templates"
        description="Pre-built job templates — select one to launch with default configuration."
      />

      <div className="flex-1 overflow-y-auto p-6">
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
                <span className="text-xs text-muted-foreground">{tpl.badge}</span>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={e => {
                    e.stopPropagation()
                    launch(tpl)
                  }}
                >
                  Use template
                </Button>
              </CardFooter>
            </Card>
          ))}
        </div>
      </div>

      <SubmitJobDialog
        key={selectedLabel ?? 'default'}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSubmitted={() => setDialogOpen(false)}
        initialTemplate={selectedLabel}
      />
    </DashboardShell>
  )
}
