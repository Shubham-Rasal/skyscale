'use client'

import { useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { LineChart, Search } from 'lucide-react'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { StatBadge } from '@/components/dashboard/stat-badge'
import { TrainingTable } from '@/components/TrainingTable'
import { SubmitJobDialog } from '@/components/SubmitJobDialog'
import { TrainingChart } from '@/components/TrainingChart'
import { GPUMetrics } from '@/components/GPUMetrics'
import { JobDetailDrawer } from '@/components/JobDetailDrawer'
import { AuthStatus } from '@/components/AuthStatus'
import { NewRlRunButton, RlRunsPanel } from '@/components/training/rl-runs-panel'
import { authClient } from '@/lib/auth-client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

const FILTERS = ['All', 'Running', 'GPU'] as const
const RL_JOB_TYPES = new Set(['training_run', 'rl_env'])

function isRlJob(jobType: string) {
  return RL_JOB_TYPES.has(jobType)
}

export default function TrainingPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const tab = searchParams.get('tab') === 'rollouts' ? 'rollouts' : 'runs'
  const { data, connected } = useRealtimeStream()
  const { data: session, isPending: sessionPending } = authClient.useSession()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [rlRunOpen, setRlRunOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [activeFilter, setActiveFilter] = useState<(typeof FILTERS)[number]>('All')
  const [showMetrics, setShowMetrics] = useState(false)
  const [selectedExec, setSelectedExec] = useState<string | null>(null)

  const activeGPU = data.vm_pool.filter(v => v.HardwareType === 'gpu').length
  const hasTrainingMetrics = Object.keys(data.training_metrics).length > 0
  const canLaunchGPU = Boolean(session)

  const rlActive = data.active.filter(e => isRlJob(e.JobType || ''))
  const rlRecent = data.recent.filter(e => isRlJob(e.JobType || ''))
  const runningRollouts = rlActive.length + rlRecent.filter(e => e.Status === 'running').length

  function setTab(next: string) {
    const params = new URLSearchParams(searchParams.toString())
    if (next === 'runs') {
      params.delete('tab')
    } else {
      params.set('tab', next)
    }
    const query = params.toString()
    router.replace(query ? `/?${query}` : '/', { scroll: false })
  }

  function openNewRun() {
    if (!canLaunchGPU) {
      router.push('/login?next=/')
      return
    }
    setDialogOpen(true)
  }

  const filterActive = (items: typeof data.active) =>
    items.filter(e => {
      if (!isRlJob(e.JobType || '')) return false
      const matchesSearch = !search || e.FunctionName?.toLowerCase().includes(search.toLowerCase())
      const matchesFilter =
        activeFilter === 'All' ||
        activeFilter === 'Running' ||
        (activeFilter === 'GPU' && e.HardwareType === 'gpu')
      return matchesSearch && matchesFilter
    })

  const filterRecent = (items: typeof data.recent) =>
    items.filter(e => {
      if (!isRlJob(e.JobType || '')) return false
      const matchesSearch = !search || e.FunctionName?.toLowerCase().includes(search.toLowerCase())
      const matchesFilter =
        activeFilter === 'All' ||
        (activeFilter === 'Running' && e.Status === 'running') ||
        (activeFilter === 'GPU' && e.HardwareType === 'gpu')
      return matchesSearch && matchesFilter
    })

  return (
    <DashboardShell connected={connected}>
      <PageHeader
        title="Training"
        description="GRPO fine-tuning runs and live rollout workers."
        badge="Beta"
        stats={
          <>
            <StatBadge label="Rollouts" value={runningRollouts} tone="running" />
            <StatBadge label="GPU nodes" value={activeGPU} tone="primary" />
            <StatBadge
              label="Done"
              value={rlRecent.filter(e => e.Status === 'completed').length}
              tone="success"
            />
          </>
        }
        actions={
          <>
            {tab === 'rollouts' && (hasTrainingMetrics || activeGPU > 0) && (
              <Button
                variant={showMetrics ? 'secondary' : 'outline'}
                size="sm"
                onClick={() => setShowMetrics(v => !v)}
              >
                <LineChart className="size-3.5" />
                Metrics
              </Button>
            )}
            <AuthStatus />
            {tab === 'runs' ? (
              <NewRlRunButton onClick={() => setRlRunOpen(true)} />
            ) : (
              <SubmitJobDialog
                open={dialogOpen}
                onOpenChange={setDialogOpen}
                onSubmitted={() => setDialogOpen(false)}
                trigger={
                  <Button size="sm" disabled={sessionPending} onClick={openNewRun}>
                    {canLaunchGPU ? 'New Rollout' : 'Sign in to run'}
                  </Button>
                }
              />
            )}
          </>
        }
        toolbar={
          tab === 'rollouts' ? (
            <div className="flex items-center gap-3">
              <div className="relative max-w-sm flex-1">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={e => setSearch(e.target.value)}
                  placeholder="Search rollouts by name…"
                  className="h-8 pl-8"
                />
              </div>
              <div className="hidden h-5 w-px bg-border sm:block" />
              <div className="flex items-center gap-1">
                {FILTERS.map(filter => (
                  <Button
                    key={filter}
                    variant={activeFilter === filter ? 'secondary' : 'ghost'}
                    size="sm"
                    className="h-7 px-2.5 text-xs"
                    onClick={() => setActiveFilter(filter)}
                  >
                    {filter}
                  </Button>
                ))}
              </div>
            </div>
          ) : undefined
        }
      />

      <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-6 pt-4">
        <Tabs value={tab} onValueChange={setTab} className="flex min-h-0 flex-1 flex-col">
          <TabsList className="mb-4 w-fit">
            <TabsTrigger value="runs">RL Runs</TabsTrigger>
            <TabsTrigger value="rollouts">Rollouts</TabsTrigger>
          </TabsList>

          <TabsContent value="runs" className="mt-0 min-h-0 flex-1 overflow-y-auto pb-6">
            <RlRunsPanel newRunOpen={rlRunOpen} onNewRunOpenChange={setRlRunOpen} />
          </TabsContent>

          <TabsContent value="rollouts" className="mt-0 flex min-h-0 flex-1 flex-col overflow-hidden data-[state=inactive]:hidden">
            {showMetrics && (
              <div
                className={cn(
                  'grid shrink-0 border-b border-border',
                  hasTrainingMetrics ? 'grid-cols-1 lg:grid-cols-2' : 'grid-cols-1',
                )}
              >
                {hasTrainingMetrics && (
                  <div className="border-b border-border p-5 lg:border-b-0 lg:border-r">
                    <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                      Training Curves
                    </p>
                    <TrainingChart trainingMetrics={data.training_metrics} />
                  </div>
                )}
                {activeGPU > 0 && (
                  <div className="p-5">
                    <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                      GPU Utilization
                    </p>
                    <GPUMetrics metrics={data.gpu} />
                  </div>
                )}
              </div>
            )}

            <TrainingTable
              active={filterActive(data.active)}
              recent={filterRecent(data.recent)}
              trainingMetrics={data.training_metrics}
              onNewRun={openNewRun}
              onSelectExecution={setSelectedExec}
            />
            <JobDetailDrawer executionId={selectedExec} onClose={() => setSelectedExec(null)} />
          </TabsContent>
        </Tabs>
      </div>
    </DashboardShell>
  )
}
