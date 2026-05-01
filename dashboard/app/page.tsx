'use client'

import { useState } from 'react'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { Sidebar } from '@/components/Sidebar'
import { TrainingTable } from '@/components/TrainingTable'
import { SubmitJobDialog } from '@/components/SubmitJobDialog'
import { TrainingChart } from '@/components/TrainingChart'
import { GPUMetrics } from '@/components/GPUMetrics'
import { JobDetailDrawer } from '@/components/JobDetailDrawer'

export default function HomePage() {
  const { data, connected } = useRealtimeStream()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [showMetrics, setShowMetrics] = useState(false)
  const [selectedExec, setSelectedExec] = useState<string | null>(null)

  const activeGPU = data.vm_pool.filter(v => v.HardwareType === 'gpu').length
  const hasTrainingMetrics = Object.keys(data.training_metrics).length > 0

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={connected} />

      {/* Main */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        {/* Top bar */}
        <div style={{
          height: 52,
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          padding: '0 24px',
          gap: 12,
          flexShrink: 0,
        }}>
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 12 }}>
            <h1 style={{
              fontFamily: "var(--font-grotesk), sans-serif",
              fontSize: 15,
              fontWeight: 600,
              color: 'var(--text-primary)',
              letterSpacing: '-0.01em',
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
              letterSpacing: '0.04em',
            }}>
              BETA
            </span>
          </div>

          {/* Stats pills */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <StatPill label="Active" value={data.active.length} color="#3b82f6" />
            <StatPill label="GPU nodes" value={activeGPU} color="var(--accent)" />
            <StatPill label="Completed" value={data.recent.filter(e => e.Status === 'completed').length} color="#22c55e" />
          </div>

          {/* Metrics toggle */}
          {(hasTrainingMetrics || activeGPU > 0) && (
            <button
              onClick={() => setShowMetrics(v => !v)}
              style={{
                padding: '5px 12px',
                background: showMetrics ? 'var(--bg-active)' : 'transparent',
                border: '1px solid var(--border-light)',
                borderRadius: 6,
                color: showMetrics ? 'var(--text-primary)' : 'var(--text-secondary)',
                fontSize: 12,
                fontWeight: 500,
                cursor: 'pointer',
                fontFamily: 'inherit',
                transition: 'all 0.1s',
              }}
            >
              Metrics
            </button>
          )}

          <SubmitJobDialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            onSubmitted={() => setDialogOpen(false)}
            trigger={
              <button
                onClick={() => setDialogOpen(true)}
                style={{
                  padding: '6px 14px',
                  background: 'var(--accent)',
                  color: 'white',
                  border: 'none',
                  borderRadius: 7,
                  fontSize: 13,
                  fontWeight: 500,
                  cursor: 'pointer',
                  fontFamily: 'inherit',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                }}
              >
                <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round">
                  <path d="M6 1v10M1 6h10"/>
                </svg>
                New Run
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
          gap: 10,
          flexShrink: 0,
        }}>
          <div style={{ position: 'relative', flex: 1, maxWidth: 320 }}>
            <svg
              width="13" height="13" viewBox="0 0 13 13" fill="none"
              stroke="var(--text-muted)" strokeWidth="1.5" strokeLinecap="round"
              style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', pointerEvents: 'none' }}
            >
              <circle cx="5.5" cy="5.5" r="4.5"/><path d="M10 10l2 2"/>
            </svg>
            <input
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Search runs..."
              style={{
                width: '100%',
                height: 32,
                paddingLeft: 30,
                paddingRight: 10,
                background: 'var(--bg-panel)',
                border: '1px solid var(--border)',
                borderRadius: 6,
                color: 'var(--text-primary)',
                fontSize: 13,
                fontFamily: 'inherit',
                outline: 'none',
              }}
              onFocus={e => (e.target as HTMLInputElement).style.borderColor = 'var(--border-light)'}
              onBlur={e => (e.target as HTMLInputElement).style.borderColor = 'var(--border)'}
            />
          </div>
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
              gap: 0,
            }}>
              {hasTrainingMetrics && (
                <div style={{ borderRight: '1px solid var(--border)', padding: 20 }}>
                  <div style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 12, letterSpacing: '0.04em', textTransform: 'uppercase' }}>
                    Training Curves
                  </div>
                  <TrainingChart trainingMetrics={data.training_metrics} />
                </div>
              )}
              {activeGPU > 0 && (
                <div style={{ padding: 20 }}>
                  <div style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 12, letterSpacing: '0.04em', textTransform: 'uppercase' }}>
                    GPU Utilization
                  </div>
                  <GPUMetrics metrics={data.gpu} />
                </div>
              )}
            </div>
          )}

          <TrainingTable
            active={data.active.filter(e => !search || e.FunctionName?.toLowerCase().includes(search.toLowerCase()))}
            recent={data.recent.filter(e => !search || e.FunctionName?.toLowerCase().includes(search.toLowerCase()))}
            trainingMetrics={data.training_metrics}
            onNewRun={() => setDialogOpen(true)}
            onSelectExecution={setSelectedExec}
          />
          <JobDetailDrawer executionId={selectedExec} onClose={() => setSelectedExec(null)} />
        </div>
      </div>
    </div>
  )
}

function StatPill({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 5,
      padding: '3px 10px',
      borderRadius: 20,
      background: 'var(--bg-panel)',
      border: '1px solid var(--border)',
      fontSize: 12,
    }}>
      <span style={{ color: 'var(--text-secondary)' }}>{label}</span>
      <span style={{ fontWeight: 600, color }}>{value}</span>
    </div>
  )
}

function FilterChip({ label, active }: { label: string; active?: boolean }) {
  return (
    <button style={{
      padding: '4px 10px',
      borderRadius: 5,
      border: active ? '1px solid var(--border-light)' : '1px solid transparent',
      background: active ? 'var(--bg-active)' : 'transparent',
      color: active ? 'var(--text-primary)' : 'var(--text-secondary)',
      fontSize: 12,
      fontWeight: active ? 500 : 400,
      cursor: 'pointer',
      fontFamily: 'inherit',
    }}>
      {label}
    </button>
  )
}
