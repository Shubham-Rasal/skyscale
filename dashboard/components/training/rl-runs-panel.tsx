'use client'

import { useState } from 'react'
import { Plus } from 'lucide-react'
import { api } from '@/lib/api'
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

interface NewRunForm {
  base_model: string
  num_workers: number
  gpu_model: string
}

const defaultForm: NewRunForm = {
  base_model: 'Qwen/Qwen3-0.6B',
  num_workers: 2,
  gpu_model: 'a10g',
}

const GPU_MODELS = ['a10g', 'a100', 'h100', 't4', 'l4', 'a6000']
const BASE_MODELS = [
  'Qwen/Qwen3-0.6B',
  'rasdani/Qwen2.5-0.5B-Open-R1-Code-GRPO',
  'Qwen/Qwen2.5-Coder-1.5B-Instruct',
  'Qwen/Qwen2.5-Coder-7B-Instruct',
  'deepseek-ai/DeepSeek-Coder-1.3B-Instruct',
]

export function NewRlRunButton({ onClick }: { onClick: () => void }) {
  return (
    <Button size="sm" onClick={onClick}>
      <Plus className="size-3.5" />
      New RL Run
    </Button>
  )
}

export function NewRunDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (runId: string) => void }) {
  const [form, setForm] = useState<NewRunForm>(defaultForm)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
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
          <div className="space-y-1.5">
            <Label>Base Model</Label>
            <Select value={form.base_model} onValueChange={v => setForm(f => ({ ...f, base_model: v }))}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {BASE_MODELS.map(m => <SelectItem key={m} value={m}>{m}</SelectItem>)}
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
          <Button onClick={submit} disabled={loading}>
            {loading ? 'Starting…' : 'Start Run'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
