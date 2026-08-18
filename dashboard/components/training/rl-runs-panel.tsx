'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Plus } from 'lucide-react'
import { api } from '@/lib/api'
import { authClient } from '@/lib/auth-client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export interface RlRunPreset {
  backend?: 'slime' | 'skyscale'
  base_model: string
  num_workers: number
  gpu_model: string
  problem_set?: string
}

type RlRunForm = RlRunPreset & { backend: 'slime' | 'skyscale' }

const defaultForm: RlRunForm = {
  backend: 'slime',
  base_model: 'Qwen/Qwen3-0.6B',
  num_workers: 2,
  gpu_model: 'a10g',
  problem_set: 'default',
}

const GPU_MODELS = ['a10g', 'a100', 'h100', 't4', 'l4', 'a6000']
const BASE_MODELS = [
  'Qwen/Qwen3-0.6B',
  'Qwen/Qwen3-1.7B',
  'Qwen/Qwen3-4B-Instruct-2507',
  'deepseek-ai/DeepSeek-R1-Distill-Qwen-1.5B',
  'rasdani/Qwen2.5-0.5B-Open-R1-Code-GRPO',
  'Qwen/Qwen2.5-Coder-1.5B-Instruct',
  'Qwen/Qwen2.5-Coder-7B-Instruct',
  'deepseek-ai/DeepSeek-Coder-1.3B-Instruct',
]

export function NewRlRunButton({ onClick }: { onClick: () => void }) {
  const router = useRouter()
  const { data: session, isPending } = authClient.useSession()

  const handleClick = () => {
    if (!session) {
      router.push('/login')
      return
    }
    onClick()
  }

  return (
    <Button size="sm" onClick={handleClick} disabled={isPending}>
      <Plus className="size-3.5" />
      New RL Run
    </Button>
  )
}

export function NewRunDialog({
  open,
  onClose,
  onCreated,
  preset,
}: {
  open: boolean
  onClose: () => void
  onCreated: (runId: string) => void
  preset?: RlRunPreset
}) {
  const { data: session, isPending } = authClient.useSession()
  const [form, setForm] = useState<RlRunForm>(() => ({ ...defaultForm, ...preset }))
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
    if (!session) {
      setError('Sign in to start GPU RL training runs.')
      return
    }
    setLoading(true)
    setError('')
    try {
      const result = await api.startRLRun(form)
      onCreated(result.run_id)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to start run')
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={v => !v && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>New RL Training Run</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {!isPending && !session && (
            <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
              <Link href="/login" className="font-medium text-primary hover:underline">
                Sign in
              </Link>{' '}
              to launch GPU RL training runs.
            </p>
          )}

          <div className="space-y-1.5">
            <Label>Runtime</Label>
            <Select
              value={form.backend}
              onValueChange={value => {
                const backend = value as 'slime' | 'skyscale'
                setForm(current => ({
                  ...current,
                  backend,
                  base_model: backend === 'slime' ? 'Qwen/Qwen3-0.6B' : current.base_model,
                }))
              }}
            >
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="slime">Slime on KubeRay</SelectItem>
                <SelectItem value="skyscale">Legacy workers</SelectItem>
              </SelectContent>
            </Select>
            {form.backend === 'slime' && (
              <p className="text-xs text-muted-foreground">
                Uses managed rollout engines, sandbox rewards, and durable checkpoints.
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label>Base Model</Label>
            <Select value={form.base_model} onValueChange={v => setForm(f => ({ ...f, base_model: v }))}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {(form.backend === 'slime' ? BASE_MODELS.slice(0, 1) : BASE_MODELS).map(m => (
                  <SelectItem key={m} value={m}>{m}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label>GPU Model</Label>
            <Select value={form.gpu_model} onValueChange={v => setForm(f => ({ ...f, gpu_model: v }))}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {GPU_MODELS.map(m => <SelectItem key={m} value={m}>{m.toUpperCase()}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="workers">Rollout Workers</Label>
            <Input
              id="workers"
              type="number"
              min={1}
              max={16}
              value={form.num_workers}
              onChange={e => setForm(f => ({ ...f, num_workers: Number(e.target.value) }))}
            />
          </div>

          {error && (
            <p className="rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button onClick={submit} disabled={loading || isPending || !session}>
            {loading ? 'Starting…' : 'Start Run'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
