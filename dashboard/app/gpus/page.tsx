'use client'

import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { Sidebar } from '@/components/Sidebar'
import { GPUMetrics } from '@/components/GPUMetrics'
import { ProviderPool } from '@/components/ProviderPool'

export default function GPUsPage() {
  const { data, connected } = useRealtimeStream()

  const activeGPU = data.vm_pool.filter(v => v.HardwareType === 'gpu').length
  const activeJobs = data.gpu.length

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={connected} />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        {/* Top bar */}
        <div style={{
          height: 56,
          borderBottom: '1px solid var(--border)',
          display: 'flex', alignItems: 'center',
          padding: '0 24px', gap: 10, flexShrink: 0,
        }}>
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 10 }}>
            <h1 style={{
              fontFamily: 'var(--font-grotesk), sans-serif',
              fontSize: 15, fontWeight: 600,
              color: 'var(--text-primary)', letterSpacing: '-0.02em',
            }}>
              On-Demand GPUs
            </h1>
            <span style={{
              fontSize: 10, fontWeight: 600,
              padding: '2px 6px', borderRadius: 4,
              background: 'var(--accent-dim)',
              color: 'var(--accent)',
              letterSpacing: '0.06em',
              border: '1px solid var(--accent-border)',
            }}>
              BETA
            </span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <StatPill label="Active Jobs" value={activeJobs} color="var(--accent)" dimColor="var(--accent-dim)" />
            <StatPill label="GPU Nodes" value={activeGPU} color="var(--warning)" dimColor="rgba(245,158,11,0.1)" />
            <StatPill label="Total VMs" value={data.vm_pool.length} color="var(--success)" dimColor="var(--success-dim)" />
          </div>
        </div>

        <div style={{ flex: 1, overflowY: 'auto', padding: '24px', display: 'flex', flexDirection: 'column', gap: 16 }}>
          {activeJobs > 0 && (
            <Panel label="Live Utilization">
              <GPUMetrics metrics={data.gpu} />
            </Panel>
          )}

          <Panel label="Provider Pool">
            <ProviderPool vms={data.vm_pool} />
          </Panel>
        </div>
      </div>
    </div>
  )
}

function Panel({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{
      background: 'var(--surface)',
      borderRadius: 12,
      border: '1px solid var(--border)',
      overflow: 'hidden',
    }}>
      <div style={{
        padding: '14px 20px',
        borderBottom: '1px solid var(--border)',
        fontSize: 11,
        fontWeight: 600,
        color: 'var(--text-secondary)',
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
      }}>
        {label}
      </div>
      <div style={{ padding: 20 }}>
        {children}
      </div>
    </div>
  )
}

function StatPill({ label, value, color, dimColor }: { label: string; value: number; color: string; dimColor: string }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 5,
      padding: '4px 10px',
      borderRadius: 6,
      background: dimColor,
      border: '1px solid var(--border)',
      fontSize: 12,
    }}>
      <span style={{ fontWeight: 600, color }}>{value}</span>
      <span style={{ color: 'var(--text-secondary)' }}>{label}</span>
    </div>
  )
}
