'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Plus, Search } from 'lucide-react'
import { api, RLRun } from '@/lib/api'
import { EmptyState } from '@/components/dashboard/empty-state'
import { StatusBadge } from '@/components/dashboard/status-badge'
import { RlHeroChart } from '@/components/training/rl-charts'
import { RlRunDetailPanel } from '@/components/training/rl-run-detail-panel'
import { NewRunDialog } from '@/components/training/rl-runs-panel'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

interface RlDashboardProps {
  newRunOpen: boolean
  onNewRunOpenChange: (open: boolean) => void
  onStatsChange?: (stats: { active: number; total: number; completed: number }) => void
}

export function RlDashboard({ newRunOpen, onNewRunOpenChange, onStatsChange }: RlDashboardProps) {
  const [runs, setRuns] = useState<RLRun[]>([])
  const [selectedRun, setSelectedRun] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [heroMetrics, setHeroMetrics] = useState<Array<{ step: number; episode_reward: number; loss: number; gpu_util: number; timestamp: number }>>([])

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

  const sortedRuns = useMemo(
    () => [...runs].sort((a, b) => new Date(b.UpdatedAt).getTime() - new Date(a.UpdatedAt).getTime()),
    [runs],
  )

  const filteredRuns = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return sortedRuns
    return sortedRuns.filter(
      (r) =>
        r.ID.toLowerCase().includes(q) ||
        r.BaseModel.toLowerCase().includes(q) ||
        r.Status.toLowerCase().includes(q),
    )
  }, [sortedRuns, query])

  const activeRuns = runs.filter((r) => r.Status === 'running' || r.Status === 'starting').length
  const completedRuns = runs.filter((r) => r.Status === 'completed').length
  const selected = runs.find((r) => r.ID === selectedRun) ?? null

  useEffect(() => {
    onStatsChange?.({ active: activeRuns, total: runs.length, completed: completedRuns })
  }, [activeRuns, completedRuns, runs.length, onStatsChange])

  useEffect(() => {
    if (selectedRun && runs.some((r) => r.ID === selectedRun)) return
    if (sortedRuns.length > 0) setSelectedRun(sortedRuns[0].ID)
    else setSelectedRun(null)
  }, [runs, selectedRun, sortedRuns])

  useEffect(() => {
    const targetId = selectedRun ?? sortedRuns[0]?.ID
    if (!targetId) {
      setHeroMetrics([])
      return
    }

    let cancelled = false
    ;(async () => {
      try {
        const detail = await api.getRLRun(targetId)
        if (!cancelled) setHeroMetrics(detail.metrics ?? [])
      } catch {
        if (!cancelled) setHeroMetrics([])
      }
    })()

    return () => { cancelled = true }
  }, [selectedRun, sortedRuns])

  const handleCreated = (runId: string) => {
    onNewRunOpenChange(false)
    setSelectedRun(runId)
    refresh()
  }

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden">
      <div className="flex min-w-0 flex-1 overflow-hidden">
        <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <div className="shrink-0 space-y-5 border-b border-border px-6 py-5">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div>
                <p className="font-mono text-xs text-muted-foreground">
                  {selected ? selected.ID : 'skyscale-rl-trainer'}
                </p>
                <h2 className="mt-1 text-2xl font-semibold tracking-tight">RL Runs</h2>
              </div>
              {selected && (
                <StatusBadge status={selected.Status} className="mt-1" />
              )}
            </div>

            <div className="grid gap-3 sm:grid-cols-3">
              {[
                { label: 'Active runs', value: activeRuns, hint: 'Running or starting' },
                { label: 'Total runs', value: runs.length, hint: `${completedRuns} completed` },
                {
                  label: 'Latest reward',
                  value: heroMetrics.length
                    ? heroMetrics[heroMetrics.length - 1].episode_reward.toFixed(3)
                    : '—',
                  hint: selected?.BaseModel.split('/').pop() ?? 'No run selected',
                },
              ].map((stat) => (
                <div
                  key={stat.label}
                  className="rounded-sm border border-border bg-card px-4 py-3"
                >
                  <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                    {stat.label}
                  </p>
                  <p className="mt-1 text-3xl font-semibold tabular-nums tracking-tight">{stat.value}</p>
                  <p className="mt-1 truncate text-xs text-muted-foreground">{stat.hint}</p>
                </div>
              ))}
            </div>

            <RlHeroChart data={heroMetrics} className="h-52" />
          </div>

          <div className="flex shrink-0 items-center gap-3 border-b border-border px-6 py-3">
            <div className="relative max-w-sm flex-1">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Filter by run ID, model, status…"
                className="h-8 pl-8 text-xs"
              />
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto">
            {loading && (
              <p className="p-6 text-sm text-muted-foreground">Loading runs…</p>
            )}

            {!loading && filteredRuns.length === 0 && (
              <EmptyState
                title="No RL runs"
                description="Start a distributed GRPO run to fine-tune a coding agent. Rollout workers are managed automatically inside each run."
                action={{ label: 'New RL Run', onClick: () => onNewRunOpenChange(true) }}
              />
            )}

            {!loading && filteredRuns.length > 0 && (
              <Table>
                <TableHeader className="sticky top-0 z-10 bg-background">
                  <TableRow className="hover:bg-transparent">
                    <TableHead>Run ID</TableHead>
                    <TableHead>Model</TableHead>
                    <TableHead>GPU</TableHead>
                    <TableHead>Workers</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead>Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredRuns.map((run) => (
                    <TableRow
                      key={run.ID}
                      className={cn(
                        'cursor-pointer',
                        selectedRun === run.ID && 'bg-accent',
                      )}
                      onClick={() => setSelectedRun(run.ID)}
                    >
                      <TableCell className="font-mono text-xs">{run.ID}</TableCell>
                      <TableCell className="max-w-[140px] truncate font-mono text-xs">
                        {run.BaseModel.split('/').pop()}
                      </TableCell>
                      <TableCell className="text-xs uppercase">{run.GPUModel}</TableCell>
                      <TableCell>{run.NumWorkers}</TableCell>
                      <TableCell><StatusBadge status={run.Status} /></TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {formatCreated(run.CreatedAt)}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {relativeTime(run.UpdatedAt)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </div>
        </div>

        {selectedRun && <RlRunDetailPanel runId={selectedRun} />}
      </div>

      <NewRunDialog
        open={newRunOpen}
        onClose={() => onNewRunOpenChange(false)}
        onCreated={handleCreated}
      />
    </div>
  )
}

function formatCreated(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const formatted = date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
  const offset = -date.getTimezoneOffset() / 60
  const sign = offset >= 0 ? '+' : ''
  return `${formatted} (GMT${sign}${offset})`
}

function relativeTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const diff = Date.now() - date.getTime()
  if (diff < 60_000) return 'just now'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}
