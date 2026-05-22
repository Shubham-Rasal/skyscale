'use client'

import { ExecutionContext, Execution } from '@/types'
import { EmptyState } from '@/components/dashboard/empty-state'
import { StatusBadge } from '@/components/dashboard/status-badge'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TrendingUp } from 'lucide-react'
import { cn } from '@/lib/utils'

interface Props {
  active: ExecutionContext[]
  recent: Execution[]
  trainingMetrics: Record<string, import('@/types').TrainingMetric[]>
  onNewRun: () => void
  onSelectExecution: (id: string) => void
}

type Row = {
  id: string
  name: string
  jobType: string
  hardwareType: string
  gpuModel: string
  status: string
  progress: number
  createdAt: string
  updatedAt: string
}

function fmtDate(s: string) {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d.getTime())) return '—'
  const diff = Date.now() - d.getTime()
  if (diff < 60000) return 'just now'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

function getProgress(jobID: string, metrics: Record<string, import('@/types').TrainingMetric[]>): number {
  const m = metrics[jobID]
  if (!m || m.length === 0) return 0
  return Math.min(100, Math.round((m[m.length - 1].step / 50000) * 100))
}

export function TrainingTable({ active, recent, trainingMetrics, onNewRun, onSelectExecution }: Props) {
  const rows: Row[] = [
    ...active.map(e => ({
      id: e.ExecutionID,
      name: e.FunctionName,
      jobType: e.JobType || 'faas_function',
      hardwareType: e.HardwareType || 'cpu',
      gpuModel: e.GPUModel || '',
      status: 'running',
      progress: getProgress(e.ExecutionID, trainingMetrics),
      createdAt: e.StartTime,
      updatedAt: e.StartTime,
    })),
    ...recent.map(e => ({
      id: e.ID,
      name: e.FunctionName,
      jobType: e.JobType || 'faas_function',
      hardwareType: e.HardwareType || 'cpu',
      gpuModel: e.GPUModel || '',
      status: e.Status,
      progress: e.Status === 'completed' ? 100 : 0,
      createdAt: e.CreatedAt,
      updatedAt: e.UpdatedAt,
    })),
  ]

  if (rows.length === 0) {
    return (
      <EmptyState
        icon={<TrendingUp className="size-5" />}
        title="No rollouts yet"
        description="Launch a rollout worker to collect trajectories for RL training."
        action={{ label: 'New Rollout', onClick: onNewRun }}
      />
    )
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <Table>
        <TableHeader className="sticky top-0 z-10 bg-background">
          <TableRow className="hover:bg-transparent">
            <TableHead className="w-[280px]">Name</TableHead>
            <TableHead className="w-[100px]">Tags</TableHead>
            <TableHead className="w-[130px]">Hardware</TableHead>
            <TableHead className="w-[110px]">Status</TableHead>
            <TableHead className="w-[160px]">Progress</TableHead>
            <TableHead className="w-[100px]">Created</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(row => {
            const isRunning = row.status === 'running'
            return (
              <TableRow
                key={row.id}
                className="cursor-pointer"
                onClick={() => onSelectExecution(row.id)}
              >
                <TableCell>
                  <div className="font-medium text-foreground">{row.name || 'Unnamed'}</div>
                  <div className="mt-0.5 font-mono text-[10px] text-muted-foreground">
                    {row.id.slice(0, 12)}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant="secondary" className="text-[11px] font-normal">
                    {row.jobType === 'training_run' ? 'Training' : row.jobType === 'rl_env' ? 'RL Env' : 'FaaS'}
                  </Badge>
                </TableCell>
                <TableCell>
                  <Badge
                    variant="outline"
                    className={cn(
                      'text-[11px] font-normal',
                      row.hardwareType === 'gpu' && 'border-primary/30 bg-primary/10 text-primary',
                    )}
                  >
                    {row.hardwareType === 'gpu' ? `GPU · ${row.gpuModel || 'a100'}` : 'CPU'}
                  </Badge>
                </TableCell>
                <TableCell>
                  <StatusBadge status={row.status} />
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                      <div
                        className={cn(
                          'h-full rounded-full transition-all duration-500',
                          isRunning && 'bg-[var(--running)]',
                          row.status === 'completed' && 'bg-[var(--success)]',
                          !isRunning && row.status !== 'completed' && 'bg-muted-foreground/40',
                        )}
                        style={{ width: `${row.progress}%` }}
                      />
                    </div>
                    <span className="w-8 text-right text-[11px] tabular-nums text-muted-foreground">
                      {row.progress}%
                    </span>
                  </div>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">{fmtDate(row.createdAt)}</TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
