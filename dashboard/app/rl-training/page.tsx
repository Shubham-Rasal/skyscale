'use client'

import { useEffect, useState, useCallback } from 'react'
import { FlaskConical, Plus } from 'lucide-react'
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from 'recharts'
import { api, RLRun, RLRunDetail } from '@/lib/api'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { EmptyState } from '@/components/dashboard/empty-state'
import { StatusBadge } from '@/components/dashboard/status-badge'
import { ContentPanel } from '@/components/dashboard/content-panel'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

interface NewRunForm {
  base_model: string
  num_workers: number
  gpu_model: string
}

const defaultForm: NewRunForm = {
  base_model: 'Qwen/Qwen3-0.6B',
  num_workers: 2,
  gpu_model: 'a100',
}

const GPU_MODELS = ['a100', 'h100', 't4', 'l4', 'a6000']
const BASE_MODELS = [
  'Qwen/Qwen3-0.6B',
  'rasdani/Qwen2.5-0.5B-Open-R1-Code-GRPO',
  'Qwen/Qwen2.5-Coder-1.5B-Instruct',
  'Qwen/Qwen2.5-Coder-7B-Instruct',
  'deepseek-ai/DeepSeek-Coder-1.3B-Instruct',
]

function NewRunDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (runId: string) => void }) {
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

function RunDetailPanel({ runId, onClose }: { runId: string; onClose: () => void }) {
  const [detail, setDetail] = useState<RLRunDetail | null>(null)

  const refresh = useCallback(async () => {
    try {
      const d = await api.getRLRun(runId)
      setDetail(d)
    } catch {}
  }, [runId])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [refresh])

  const stop = async () => {
    await api.stopRLRun(runId)
    refresh()
  }

  return (
    <Sheet open onOpenChange={v => !v && onClose()}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-lg">
        <SheetHeader className="mb-4">
          <div className="flex items-center justify-between gap-2 pr-10">
            <SheetTitle>Run Detail</SheetTitle>
            {detail && (detail.run.Status === 'running' || detail.run.Status === 'starting') && (
              <Button variant="destructive" size="sm" onClick={stop}>Stop</Button>
            )}
          </div>
        </SheetHeader>

        {!detail && <p className="text-sm text-muted-foreground">Loading…</p>}

        {detail && (
          <div className="space-y-4">
            <ContentPanel title="Run Info" contentClassName="space-y-3 p-4">
              <DetailRow label="Run ID" value={detail.run.ID} mono />
              <DetailRow label="Status" value={<StatusBadge status={detail.run.Status} />} />
              <DetailRow label="Base Model" value={detail.run.BaseModel} mono />
              <DetailRow label="GPU Model" value={detail.run.GPUModel.toUpperCase()} />
              <DetailRow label="Workers" value={String(detail.run.NumWorkers)} />
              <DetailRow label="Buffer" value={`${detail.buffer_size} trajectories`} />
            </ContentPanel>

            <ContentPanel title="Workers" contentClassName="space-y-2 p-4">
              {detail.worker_statuses.length === 0 ? (
                <p className="text-xs text-muted-foreground">No workers yet.</p>
              ) : (
                detail.worker_statuses.map((w, i) => (
                  <div key={w.id} className="flex items-center justify-between">
                    <span className="font-mono text-xs text-muted-foreground">worker-{i}</span>
                    <StatusBadge status={w.status} />
                  </div>
                ))
              )}
              {detail.trainer_status && (
                <div className="mt-2 flex items-center justify-between border-t border-border pt-2">
                  <span className="text-xs text-muted-foreground">trainer</span>
                  <StatusBadge status={detail.trainer_status} />
                </div>
              )}
            </ContentPanel>

            <ContentPanel title="Reward Curve" contentClassName="p-4">
              {detail.metrics && detail.metrics.length > 0 ? (
                <div className="h-40">
                  <ResponsiveContainer width="100%" height="100%">
                    <LineChart data={detail.metrics} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
                      <XAxis dataKey="step" tick={{ fontSize: 10, fill: 'var(--muted-foreground)' }} />
                      <YAxis tick={{ fontSize: 10, fill: 'var(--muted-foreground)' }} domain={[0, 1]} />
                      <Tooltip contentStyle={{ background: 'var(--popover)', border: '1px solid var(--border)', borderRadius: 6, fontSize: 11 }} />
                      <Legend wrapperStyle={{ fontSize: 11 }} />
                      <Line type="monotone" dataKey="episode_reward" stroke="var(--primary)" dot={false} strokeWidth={1.5} name="Reward" />
                      <Line type="monotone" dataKey="loss" stroke="var(--destructive)" dot={false} strokeWidth={1.5} name="Loss" />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              ) : (
                <p className="text-xs text-muted-foreground">No metrics yet — workers are collecting trajectories.</p>
              )}
            </ContentPanel>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}

export default function RLTrainingPage() {
  const [runs, setRuns] = useState<RLRun[]>([])
  const [showNewRun, setShowNewRun] = useState(false)
  const [selectedRun, setSelectedRun] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      const data = await api.listRLRuns()
      setRuns(Array.isArray(data) ? data : [])
    } catch {}
    setLoading(false)
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 8000)
    return () => clearInterval(t)
  }, [refresh])

  const handleCreated = (runId: string) => {
    setShowNewRun(false)
    setSelectedRun(runId)
    refresh()
  }

  return (
    <DashboardShell connected={false}>
      <PageHeader
        title="RL Training"
        description="Distributed GRPO fine-tuning for coding agents."
        actions={
          <Button size="sm" onClick={() => setShowNewRun(true)}>
            <Plus className="size-3.5" />
            New Run
          </Button>
        }
      />

      <div className="flex-1 overflow-y-auto p-6">
        {loading && <p className="text-sm text-muted-foreground">Loading…</p>}

        {!loading && runs.length === 0 && (
          <EmptyState
            icon={<FlaskConical className="size-5" />}
            title="No RL runs yet"
            description="Start a distributed RL training run to fine-tune a coding agent using GRPO."
            action={{ label: 'Start your first run', onClick: () => setShowNewRun(true) }}
          />
        )}

        {!loading && runs.length > 0 && (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>Run ID</TableHead>
                  <TableHead>Model</TableHead>
                  <TableHead>GPU</TableHead>
                  <TableHead>Workers</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map(run => (
                  <TableRow
                    key={run.ID}
                    className="cursor-pointer"
                    onClick={() => setSelectedRun(run.ID)}
                  >
                    <TableCell className="font-mono text-xs text-primary">{run.ID}</TableCell>
                    <TableCell className="font-mono text-xs">{run.BaseModel.split('/').pop()}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className="border-primary/30 bg-primary/10 text-primary">
                        {run.GPUModel.toUpperCase()}
                      </Badge>
                    </TableCell>
                    <TableCell>{run.NumWorkers}</TableCell>
                    <TableCell><StatusBadge status={run.Status} /></TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(run.CreatedAt).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      <NewRunDialog open={showNewRun} onClose={() => setShowNewRun(false)} onCreated={handleCreated} />
      {selectedRun && <RunDetailPanel runId={selectedRun} onClose={() => setSelectedRun(null)} />}
    </DashboardShell>
  )
}

function DetailRow({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="w-24 shrink-0 text-xs text-muted-foreground">{label}</span>
      {typeof value === 'string' ? (
        <span className={mono ? 'break-all font-mono text-xs text-foreground' : 'text-sm text-foreground'}>
          {value || '—'}
        </span>
      ) : value}
    </div>
  )
}
