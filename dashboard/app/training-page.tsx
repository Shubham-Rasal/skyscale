'use client'

import { useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { StatBadge } from '@/components/dashboard/stat-badge'
import { AuthStatus } from '@/components/AuthStatus'
import { NewRlRunButton } from '@/components/training/rl-runs-panel'
import { RlDashboard } from '@/components/training/rl-dashboard'

export default function TrainingPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { connected, data } = useRealtimeStream()
  const [rlRunOpen, setRlRunOpen] = useState(false)
  const [runStats, setRunStats] = useState({ active: 0, total: 0, completed: 0 })

  const activeGPU = data.vm_pool.filter(v => v.HardwareType === 'gpu').length

  useEffect(() => {
    if (searchParams.get('tab') === 'rollouts') {
      router.replace('/lab', { scroll: false })
    }
  }, [searchParams, router])

  return (
    <DashboardShell connected={connected}>
      <PageHeader
        title="Training"
        description="Distributed GRPO fine-tuning with managed rollout workers."
        badge="Beta"
        stats={
          <>
            <StatBadge label="Active" value={runStats.active} tone="running" />
            <StatBadge label="GPU nodes" value={activeGPU} tone="primary" />
            <StatBadge label="Completed" value={runStats.completed} tone="success" />
          </>
        }
        actions={
          <>
            <AuthStatus />
            <NewRlRunButton onClick={() => setRlRunOpen(true)} />
          </>
        }
      />

      <RlDashboard
        newRunOpen={rlRunOpen}
        onNewRunOpenChange={setRlRunOpen}
        onStatsChange={setRunStats}
      />
    </DashboardShell>
  )
}
