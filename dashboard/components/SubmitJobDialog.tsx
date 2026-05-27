'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { api } from '@/lib/api'
import { authClient } from '@/lib/auth-client'

const TEMPLATES = [
  {
    label: 'MNIST CNN',
    defaults: {
      functionName: 'mnist-trainer',
      jobType: 'training_run',
      hardwareType: 'gpu',
      provider: 'akash',
      gpuModel: 'a100',
      dockerImage: 'ghcr.io/shubham-rasal/skyscale-mnist:v1',
      controlPlaneURL: 'http://n8n.maximalstudio.in:8080',
      input: JSON.stringify({ epochs: 3, batch_size: 512 }, null, 2),
    },
  },
  {
    label: 'MNIST HF',
    defaults: {
      functionName: 'hf-mnist-trainer',
      jobType: 'training_run',
      hardwareType: 'gpu',
      provider: 'huggingface',
      gpuModel: 'a10g',
      dockerImage: 'ghcr.io/shubham-rasal/skyscale-mnist:v1',
      controlPlaneURL: 'http://n8n.maximalstudio.in:8080',
      input: JSON.stringify({ epochs: 1, batch_size: 512 }, null, 2),
    },
  },
  {
    label: 'CartPole GPU',
    defaults: {
      functionName: 'cartpole-trainer',
      jobType: 'training_run',
      hardwareType: 'gpu',
      provider: 'akash',
      gpuModel: 'a100',
      dockerImage: 'ghcr.io/shubham-rasal/skyscale/skyscale-trainer:latest',
      controlPlaneURL: 'http://n8n.maximalstudio.in:8080',
      input: JSON.stringify({ total_steps: 50000, report_every: 1000 }, null, 2),
    },
  },
  {
    label: 'CPU FaaS',
    defaults: {
      functionName: '',
      jobType: 'faas_function',
      hardwareType: 'cpu',
      provider: 'akash',
      gpuModel: '',
      dockerImage: '',
      controlPlaneURL: '',
      input: '{}',
    },
  },
  {
    label: 'Custom',
    defaults: {
      functionName: '',
      jobType: 'faas_function',
      hardwareType: 'cpu',
      provider: 'akash',
      gpuModel: '',
      dockerImage: '',
      controlPlaneURL: '',
      input: '{}',
    },
  },
]

interface Props {
  open?: boolean
  onOpenChange?: (o: boolean) => void
  onSubmitted?: () => void
  trigger?: React.ReactNode
  initialTemplate?: string // label of a TEMPLATES entry to pre-select
}

const GPU_MODELS: Record<string, { value: string; label: string }[]> = {
  akash: [
    { value: 'a100', label: 'A100 80GB (SXM4)' },
    { value: 'h100', label: 'H100 80GB (SXM5)' },
    { value: 'h200', label: 'H200 141GB (SXM5)' },
    { value: 'rtx4090', label: 'RTX 4090 24GB' },
    { value: 'rtx3090', label: 'RTX 3090 24GB' },
    { value: 'rtx3060', label: 'RTX 3060 12GB' },
    { value: 'rtx6000', label: 'RTX 6000 24GB' },
    { value: 't4', label: 'T4 16GB' },
  ],
  huggingface: [
    { value: 'a10g', label: 'A10G 24GB' },
    { value: 'a10g-large', label: 'A10G 24GB Large' },
    { value: 'a100', label: 'A100 80GB' },
    { value: 'h200', label: 'H200 141GB' },
    { value: 'l4', label: 'L4 24GB' },
    { value: 't4', label: 'T4 16GB' },
  ],
}

function templateIndex(label?: string) {
  if (!label) return 0
  const idx = TEMPLATES.findIndex(t => t.label === label)
  return idx >= 0 ? idx : 0
}

function isLocalCallbackURL(value: string) {
  try {
    const host = new URL(value).hostname.toLowerCase()
    if (!host || host === 'localhost' || host === '0.0.0.0' || host === '::1') return true
    if (host.startsWith('127.') || host.startsWith('10.') || host.startsWith('192.168.')) return true
    const parts = host.split('.').map(Number)
    return parts.length === 4 && parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31
  } catch {
    return true
  }
}

export function SubmitJobDialog({ open, onOpenChange, onSubmitted, trigger, initialTemplate }: Props) {
  const { data: session, isPending } = authClient.useSession()
  const initIdx = templateIndex(initialTemplate)
  const [tplIdx, setTplIdx] = useState(initIdx)
  const [fields, setFields] = useState(TEMPLATES[initIdx].defaults)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  function applyTemplate(idx: number) {
    setTplIdx(idx)
    setFields(TEMPLATES[idx].defaults)
    setError('')
  }

  function set(k: string, v: string) {
    setFields(f => ({ ...f, [k]: v }))
  }

  function setProvider(provider: string) {
    setFields(f => ({
      ...f,
      provider,
      gpuModel: GPU_MODELS[provider]?.[0]?.value ?? f.gpuModel,
    }))
  }

  async function submit() {
    setError('')
    const isGpuJob =
      fields.hardwareType === 'gpu' ||
      fields.jobType === 'training_run'
    if (isGpuJob && !session) {
      setError('Sign in to launch GPU training jobs.')
      return
    }
    let input: Record<string, unknown> = {}
    try { input = JSON.parse(fields.input) } catch {
      setError('Input must be valid JSON')
      return
    }
    if (!fields.functionName && fields.hardwareType !== 'gpu') { setError('Function name is required'); return }
    if (fields.hardwareType === 'gpu' && fields.provider === 'huggingface' && isLocalCallbackURL(fields.controlPlaneURL)) {
      setError('Hugging Face jobs need a public Control Plane URL for metrics and completion callbacks')
      return
    }
    setSubmitting(true)
    try {
      if (fields.hardwareType === 'gpu' && fields.jobType === 'training_run') {
        // GPU training jobs go through the selected provider path.
        await api.submitTrainingJob({
          job_id: fields.functionName,
          docker_image: fields.dockerImage,
          provider: fields.provider,
          gpu_model: fields.gpuModel || 'a100',
          control_plane_url: fields.controlPlaneURL,
          env_vars: Object.fromEntries(
            Object.entries(input as Record<string, unknown>)
              .map(([k, v]) => [k.toUpperCase(), String(v)])
          ),
        })
      } else {
        if (!fields.functionName) { setError('Function name is required'); setSubmitting(false); return }
        await api.invoke(fields.functionName, {
          input,
          sync: false,
          job_type: fields.jobType,
          hardware_type: fields.hardwareType,
        })
      }
      onSubmitted?.()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Request failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger}
      <DialogContent className="max-w-md gap-4">
        <DialogHeader>
          <DialogTitle>New Training Run</DialogTitle>
        </DialogHeader>

        <Tabs value={String(tplIdx)} onValueChange={v => applyTemplate(Number(v))}>
          <TabsList className="grid h-auto w-full grid-cols-3 gap-1 bg-muted/50 p-1">
            {TEMPLATES.map((t, i) => (
              <TabsTrigger key={t.label} value={String(i)} className="text-xs">
                {t.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="functionName">Function Name *</Label>
            <Input
              id="functionName"
              value={fields.functionName}
              onChange={e => set('functionName', e.target.value)}
              placeholder="my-trainer"
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label>Job Type</Label>
              <Select value={fields.jobType} onValueChange={v => set('jobType', v)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="faas_function">FaaS Function</SelectItem>
                  <SelectItem value="training_run">Training Run</SelectItem>
                  <SelectItem value="rl_env">RL Environment</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>Hardware</Label>
              <Select value={fields.hardwareType} onValueChange={v => set('hardwareType', v)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="cpu">CPU (Firecracker)</SelectItem>
                  <SelectItem value="gpu">GPU</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          {fields.hardwareType === 'gpu' && (
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label>Provider</Label>
                <Select value={fields.provider} onValueChange={setProvider}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="akash">Akash</SelectItem>
                    <SelectItem value="huggingface">Hugging Face</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label>GPU Model</Label>
                <Select value={fields.gpuModel} onValueChange={v => set('gpuModel', v)}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {(GPU_MODELS[fields.provider] ?? GPU_MODELS.akash).map(model => (
                      <SelectItem key={model.value} value={model.value}>{model.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="col-span-2 space-y-1.5">
                <Label htmlFor="dockerImage">Docker Image</Label>
                <Input
                  id="dockerImage"
                  value={fields.dockerImage}
                  onChange={e => set('dockerImage', e.target.value)}
                  placeholder="ghcr.io/…"
                />
              </div>
            </div>
          )}

          <div className="space-y-1.5">
            <Label htmlFor="input">Input (JSON)</Label>
            <Textarea
              id="input"
              value={fields.input}
              onChange={e => set('input', e.target.value)}
              className="min-h-20 font-mono text-xs"
              placeholder="{}"
            />
          </div>

          {fields.hardwareType === 'gpu' && !isPending && !session && (
            <p className="text-sm text-muted-foreground">
              <Link href="/login" className="font-medium text-primary hover:underline">
                Sign in
              </Link>{' '}
              to launch GPU training jobs.
            </p>
          )}

          {error && (
            <p className="text-sm text-destructive">{error}</p>
          )}

          <Button
            className="w-full"
            onClick={submit}
            disabled={submitting || isPending || ((fields.hardwareType === 'gpu' || fields.jobType === 'training_run') && !session)}
          >
            {submitting ? 'Submitting…' : 'Submit Run'}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
