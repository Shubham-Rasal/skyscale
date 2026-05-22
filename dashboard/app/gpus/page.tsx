'use client'

import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { StatBadge } from '@/components/dashboard/stat-badge'
import { ContentPanel } from '@/components/dashboard/content-panel'
import { GPUMetrics } from '@/components/GPUMetrics'
import { ProviderPool } from '@/components/ProviderPool'

export default function GPUsPage() {
  const { data, connected } = useRealtimeStream()

  const activeGPU = data.vm_pool.filter(v => v.HardwareType === 'gpu').length
  const activeJobs = data.gpu.length

  return (
    <DashboardShell connected={connected}>
      <PageHeader
        title="On-Demand GPUs"
        badge="Beta"
        stats={
          <>
            <StatBadge label="Active Jobs" value={activeJobs} tone="primary" />
            <StatBadge label="GPU Nodes" value={activeGPU} tone="warning" />
            <StatBadge label="Total VMs" value={data.vm_pool.length} tone="success" />
          </>
        }
      />

      <div className="flex-1 space-y-4 overflow-y-auto p-6">
        {activeJobs > 0 && (
          <ContentPanel title="Live Utilization">
            <GPUMetrics metrics={data.gpu} />
          </ContentPanel>
        )}

        <ContentPanel title="Provider Pool">
          <ProviderPool vms={data.vm_pool} />
        </ContentPanel>
      </div>
    </DashboardShell>
  )
}
