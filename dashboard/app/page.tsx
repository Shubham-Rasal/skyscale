'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { Sidebar } from '@/components/Sidebar'
import { TrainingTable } from '@/components/TrainingTable'
import { SubmitJobDialog } from '@/components/SubmitJobDialog'
import { TrainingChart } from '@/components/TrainingChart'
import { GPUMetrics } from '@/components/GPUMetrics'
import { JobDetailDrawer } from '@/components/JobDetailDrawer'
import { AuthStatus } from '@/components/AuthStatus'
import { authClient } from '@/lib/auth-client'

export default function HomePage() {
  const router = useRouter()
  const { data, connected } = useRealtimeStream()
  const { data: session, isPending: sessionPending } = authClient.useSession()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [search, setSearch] = useState('')
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

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={connected} />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        {/* Top bar */}
        <div style={{
          height: 56,
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          padding: '0 24px',
          gap: 12,
          flexShrink: 0,
        }}>
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 10 }}>
            <h1 style={{
              fontFamily: 'var(--font-grotesk), sans-serif',
              fontSize: 15,
              fontWeight: 600,
              color: 'var(--text-primary)',
              letterSpacing: '-0.02em',
            }}>
              Training
            </h1>
            <span style={{
              fontSize: 10,
              fontWeight: 600,
              padding: '2px 6px',
              borderRadius: 4,
              background: 'var(--accent-dim)',
              color: 'var(--accent)',
              letterSpacing: '0.06em',
              border: '1px solid var(--accent-border)',
            }}>
              BETA
            </span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <StatPill label="Active" value={data.active.length} color="var(--running)" dimColor="var(--running-dim)" />
            <StatPill label="GPU nodes" value={activeGPU} color="var(--accent)" dimColor="var(--accent-dim)" />
            <StatPill label="Done" value={data.recent.filter(e => e.Status === 'completed').length} color="var(--success)" dimColor="var(--success-dim)" />
          </div>

          {(hasTrainingMetrics || activeGPU > 0) && (
            <button
              onClick={() => setShowMetrics(v => !v)}
              style={{
                padding: '5px 12px',
                background: showMetrics ? 'var(--bg-active)' : 'transparent',
                border: `1px solid ${showMetrics ? 'var(--border-light)' : 'var(--border)'}`,
                borderRadius: 7,
                color: showMetrics ? 'var(--text-primary)' : 'var(--text-secondary)',
                fontSize: 12,
                fontWeight: 500,
                cursor: 'pointer',
                fontFamily: 'inherit',
                transition: 'all 0.1s',
                display: 'flex',
                alignItems: 'center',
                gap: 5,
              }}
            >
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
                <polyline points="1,9 4,5 7,7 11,2"/>
              </svg>
              Metrics
            </button>
          )}

          <AuthStatus />

          <SubmitJobDialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            onSubmitted={() => setDialogOpen(false)}
            trigger={
              <button
                onClick={openNewRun}
                disabled={sessionPending}
                style={{
                  padding: '7px 14px',
                  background: sessionPending ? 'rgba(124,92,252,0.5)' : 'var(--accent)',
                  color: 'white',
                  border: 'none',
                  borderRadius: 7,
                  fontSize: 13,
                  fontWeight: 500,
                  cursor: sessionPending ? 'not-allowed' : 'pointer',
                  fontFamily: 'inherit',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  letterSpacing: '-0.01em',
                  transition: 'background 0.1s',
                }}
                onMouseEnter={e => { if (!sessionPending) (e.currentTarget as HTMLButtonElement).style.background = 'var(--accent-hover)' }}
                onMouseLeave={e => { if (!sessionPending) (e.currentTarget as HTMLButtonElement).style.background = 'var(--accent)' }}
              >
                <svg width="11" height="11" viewBox="0 0 11 11" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round">
                  <path d="M5.5 1v9M1 5.5h9"/>
                </svg>
                {canLaunchGPU ? 'New Run' : 'Sign in to run'}
              </button>
            }
          />
        </div>

        {/* Search + filter bar */}
        <div style={{
          padding: '10px 24px',
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          flexShrink: 0,
          background: 'var(--bg)',
        }}>
          <div style={{ position: 'relative', flex: 1, maxWidth: 360 }}>
            <svg
              width="13" height="13" viewBox="0 0 13 13" fill="none"
              stroke="var(--text-muted)" strokeWidth="1.5" strokeLinecap="round"
              style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' }}
            >
              <circle cx="5.5" cy="5.5" r="4.5"/><path d="M10.5 10.5l1.5 1.5"/>
            </svg>
            <input
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Search by name, model, or tag…"
              style={{
                width: '100%',
                height: 33,
                paddingLeft: 30,
                paddingRight: 10,
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                borderRadius: 7,
                color: 'var(--text-primary)',
                fontSize: 13,
                fontFamily: 'inherit',
                outline: 'none',
                transition: 'border-color 0.1s',
              }}
              onFocus={e => (e.target as HTMLInputElement).style.borderColor = 'var(--border-light)'}
              onBlur={e => (e.target as HTMLInputElement).style.borderColor = 'var(--border)'}
            />
          </div>
          <div style={{ width: 1, height: 20, background: 'var(--border)', marginLeft: 2, marginRight: 2 }} />
          <FilterChip active label="All" />
          <FilterChip label="Running" />
          <FilterChip label="GPU" />
        </div>

        {/* Body */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          {showMetrics && (
            <div style={{
              borderBottom: '1px solid var(--border)',
              display: 'grid',
              gridTemplateColumns: hasTrainingMetrics ? '1fr 1fr' : '1fr',
            }}>
              {hasTrainingMetrics && (
                <div style={{ borderRight: '1px solid var(--border)', padding: '20px 24px' }}>
                  <SectionLabel>Training Curves</SectionLabel>
                  <TrainingChart trainingMetrics={data.training_metrics} />
                </div>
              )}
              {activeGPU > 0 && (
                <div style={{ padding: '20px 24px' }}>
                  <SectionLabel>GPU Utilization</SectionLabel>
                  <GPUMetrics metrics={data.gpu} />
                </div>
              )}
            </div>
          )}

          <TrainingTable
            active={data.active.filter(e => !search || e.FunctionName?.toLowerCase().includes(search.toLowerCase()))}
            recent={data.recent.filter(e => !search || e.FunctionName?.toLowerCase().includes(search.toLowerCase()))}
            trainingMetrics={data.training_metrics}
            onNewRun={openNewRun}
            onSelectExecution={setSelectedExec}
          />
          <JobDetailDrawer executionId={selectedExec} onClose={() => setSelectedExec(null)} />
        </div>
      </div>
    </div>
  )
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div style={{
      fontSize: 10,
      fontWeight: 600,
      color: 'var(--text-secondary)',
      marginBottom: 14,
      letterSpacing: '0.08em',
      textTransform: 'uppercase',
    }}>
      {children}
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

function FilterChip({ label, active }: { label: string; active?: boolean }) {
  return (
    <button style={{
      padding: '4px 10px',
      borderRadius: 6,
      border: active ? '1px solid var(--border-light)' : '1px solid transparent',
      background: active ? 'var(--bg-active)' : 'transparent',
      color: active ? 'var(--text-primary)' : 'var(--text-secondary)',
      fontSize: 12,
      fontWeight: active ? 500 : 400,
      cursor: 'pointer',
      fontFamily: 'inherit',
      transition: 'all 0.1s',
    }}>
      {label}
    </button>
  )
}
