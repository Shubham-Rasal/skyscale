'use client'

import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { Sidebar } from '@/components/Sidebar'
import { GPUMetrics } from '@/components/GPUMetrics'
import { ProviderPool } from '@/components/ProviderPool'
import { Zap, Server, Activity } from 'lucide-react'

export default function GPUsPage() {
  const { data, connected } = useRealtimeStream()

  const activeGPU = data.vm_pool.filter(v => v.HardwareType === 'gpu').length
  const activeJobs = data.gpu.length

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={connected} />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        <div style={{
          height: 52,
          borderBottom: '1px solid var(--border)',
          display: 'flex', alignItems: 'center',
          padding: '0 24px', gap: 12, flexShrink: 0,
        }}>
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 12 }}>
            <h1 style={{
              fontFamily: 'var(--font-grotesk), sans-serif',
              fontSize: 15, fontWeight: 600,
              color: 'var(--text-primary)', letterSpacing: '-0.01em',
            }}>
              On-Demand GPUs
            </h1>
            <span style={{
              fontSize: 10,
              fontWeight: 600,
              padding: '2px 6px',
              borderRadius: 4,
              background: 'var(--accent-dim)',
              color: 'var(--accent)',
              letterSpacing: '0.04em',
            }}>
              BETA
            </span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <StatPill label="Active Jobs" value={activeJobs} color="var(--accent)" />
            <StatPill label="GPU Nodes" value={activeGPU} color="#f59e0b" />
            <StatPill label="Total VMs" value={data.vm_pool.length} color="#22c55e" />
          </div>
        </div>

        <div style={{ flex: 1, overflowY: 'auto', padding: 24, display: 'flex', flexDirection: 'column', gap: 24 }}>
          {activeJobs > 0 && (
            <div style={{ background: 'var(--surface)', borderRadius: 12, border: '1px solid var(--border)', padding: 20 }}>
              <div style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 16, letterSpacing: '0.04em', textTransform: 'uppercase', display: 'flex', alignItems: 'center', gap: 6 }}>
                <Activity size={12} /> Live Utilization
              </div>
              <GPUMetrics metrics={data.gpu} />
            </div>
          )}

          <div style={{ background: 'var(--surface)', borderRadius: 12, border: '1px solid var(--border)', padding: 20 }}>
            <div style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 16, letterSpacing: '0.04em', textTransform: 'uppercase', display: 'flex', alignItems: 'center', gap: 6 }}>
              <Server size={12} /> Provider Pool
            </div>
            <ProviderPool vms={data.vm_pool} />
          </div>
        </div>
      </div>
    </div>
  )
}

function StatPill({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 5,
      padding: '4px 10px',
      background: 'var(--bg-active)',
      borderRadius: 6,
      border: '1px solid var(--border-light)',
    }}>
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: color }} />
      <span style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-primary)' }}>{value}</span>
      <span style={{ fontSize: 11, color: 'var(--text-secondary)' }}>{label}</span>
    </div>
  )
}
