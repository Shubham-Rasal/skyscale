'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
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
import { authClient } from '@/lib/auth-client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

const FILTERS = ['All', 'Running', 'GPU'] as const

export default function HomePage() {
  const router = useRouter()
  const { data, connected } = useRealtimeStream()
  const { data: session, isPending: sessionPending } = authClient.useSession()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [activeFilter, setActiveFilter] = useState<(typeof FILTERS)[number]>('All')
  const [showMetrics, setShowMetrics] = useState(false)
  const [selectedExec, setSelectedExec] = useState<string | null>(null)

  const activeGPU = data.vm_pool.filter(v => v.HardwareType === 'gpu').length
  const hasTrainingMetrics = Object.keys(data.training_metrics).length > 0
  const canLaunchGPU = Boolean(session)

  function openNewRun() {
    if (!canLaunchGPU) {
      router.push('/login?next=/')
      return
    }
    setDialogOpen(true)
  }

  const filterActive = (items: typeof data.active) =>
    items.filter(e => {
      const matchesSearch = !search || e.FunctionName?.toLowerCase().includes(search.toLowerCase())
      const matchesFilter =
        activeFilter === 'All' ||
        (activeFilter === 'Running') ||
        (activeFilter === 'GPU' && e.HardwareType === 'gpu')
      return matchesSearch && matchesFilter
    })

  const filterRecent = (items: typeof data.recent) =>
    items.filter(e => {
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
        badge="Beta"
        stats={
          <>
            <StatBadge label="Active" value={data.active.length} tone="running" />
            <StatBadge label="GPU nodes" value={activeGPU} tone="primary" />
            <StatBadge
              label="Done"
              value={data.recent.filter(e => e.Status === 'completed').length}
              tone="success"
            />
          </>
        }
        actions={
          <>
            {(hasTrainingMetrics || activeGPU > 0) && (
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
            <SubmitJobDialog
              open={dialogOpen}
              onOpenChange={setDialogOpen}
              onSubmitted={() => setDialogOpen(false)}
              trigger={
                <Button size="sm" disabled={sessionPending} onClick={openNewRun}>
                  {canLaunchGPU ? 'New Run' : 'Sign in to run'}
                </Button>
              }
            />
          </>
        }
        toolbar={
          <div className="flex items-center gap-3">
            <div className="relative max-w-sm flex-1">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={e => setSearch(e.target.value)}
                placeholder="Search by name, model, or tag…"
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
        }
      />

      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
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
      </div>
    </DashboardShell>
  )
}
