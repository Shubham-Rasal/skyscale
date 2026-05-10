'use client'

import { ExecutionContext, Execution } from '@/types'

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

function statusStyle(s: string): { dot: string; label: string; bg: string; border: string } {
  switch (s) {
    case 'running':   return { dot: 'var(--running)',  label: 'var(--running)',  bg: 'var(--running-dim)',  border: 'rgba(59,130,246,0.2)' }
    case 'queued':    return { dot: 'var(--queued)',   label: 'var(--queued)',   bg: 'rgba(163,163,163,0.08)', border: 'rgba(163,163,163,0.15)' }
    case 'completed': return { dot: 'var(--success)',  label: 'var(--success)',  bg: 'var(--success-dim)', border: 'rgba(34,197,94,0.2)' }
    case 'failed':
    case 'error':     return { dot: 'var(--error)',    label: 'var(--error)',    bg: 'var(--error-dim)',   border: 'rgba(239,68,68,0.2)' }
    default:          return { dot: 'var(--text-muted)', label: 'var(--text-secondary)', bg: 'rgba(107,107,107,0.08)', border: 'rgba(107,107,107,0.15)' }
  }
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

const COLS = [
  { label: 'Name', width: 'auto' },
  { label: 'Tags', width: 100 },
  { label: 'Hardware', width: 130 },
  { label: 'Status', width: 110 },
  { label: 'Progress', width: 140 },
  { label: 'Created', width: 100 },
]

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
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
        {/* Table header even when empty */}
        <TableHeader />
        {/* Empty state */}
        <div style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 16,
          padding: 48,
        }}>
          <div style={{
            width: 52,
            height: 52,
            border: '1px solid var(--border-light)',
            borderRadius: 12,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: 'var(--surface)',
          }}>
            <svg width="22" height="22" viewBox="0 0 22 22" fill="none" stroke="var(--text-secondary)" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
              <path d="M3 17 8 10l5 4 5-7"/>
              <circle cx="17.5" cy="6.5" r="2" fill="var(--surface)" />
            </svg>
          </div>
          <div style={{ textAlign: 'center' }}>
            <div style={{
              fontFamily: 'var(--font-grotesk), sans-serif',
              fontSize: 15,
              fontWeight: 600,
              color: 'var(--text-primary)',
              marginBottom: 6,
              letterSpacing: '-0.01em',
            }}>
              Run your first training job
            </div>
            <div style={{ fontSize: 13, color: 'var(--text-secondary)', maxWidth: 300, lineHeight: 1.6 }}>
              Submit a GPU training job and watch metrics stream live to your dashboard.
            </div>
          </div>
          <button
            onClick={onNewRun}
            style={{
              padding: '8px 18px',
              background: 'var(--bg-active)',
              color: 'var(--text-primary)',
              border: '1px solid var(--border-light)',
              borderRadius: 8,
              fontSize: 13,
              fontWeight: 500,
              cursor: 'pointer',
              fontFamily: 'inherit',
              transition: 'background 0.1s',
              letterSpacing: '-0.01em',
            }}
            onMouseEnter={e => (e.currentTarget as HTMLButtonElement).style.background = 'var(--bg-hover)'}
            onMouseLeave={e => (e.currentTarget as HTMLButtonElement).style.background = 'var(--bg-active)'}
          >
            New Run
          </button>
        </div>
      </div>
    )
  }

  return (
    <div style={{ flex: 1, overflowY: 'auto' }}>
      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead style={{ position: 'sticky', top: 0, zIndex: 2, background: 'var(--bg)' }}>
          <tr style={{ borderBottom: '1px solid var(--border)' }}>
            <th style={{ width: 36, padding: '9px 8px 9px 16px' }}>
              <div style={{
                width: 14, height: 14,
                border: '1px solid var(--border-light)',
                borderRadius: 3,
                background: 'var(--surface)',
              }} />
            </th>
            {COLS.map(col => (
              <th
                key={col.label}
                style={{
                  padding: '9px 12px',
                  textAlign: 'left',
                  fontSize: 11,
                  fontWeight: 500,
                  color: 'var(--text-secondary)',
                  letterSpacing: '0.02em',
                  whiteSpace: 'nowrap',
                  width: col.width === 'auto' ? undefined : col.width,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  {col.label}
                  <SortIcon />
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map(row => {
            const sc = statusStyle(row.status)
            const isRunning = row.status === 'running'
            return (
              <tr
                key={row.id}
                onClick={() => onSelectExecution(row.id)}
                style={{
                  borderBottom: '1px solid var(--border)',
                  cursor: 'pointer',
                  transition: 'background 0.08s',
                }}
                onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'}
                onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'transparent'}
              >
                {/* Checkbox */}
                <td style={{ padding: '11px 8px 11px 16px' }} onClick={e => e.stopPropagation()}>
                  <div style={{
                    width: 14, height: 14,
                    border: '1px solid var(--border-light)',
                    borderRadius: 3,
                    background: 'var(--surface)',
                    cursor: 'pointer',
                  }} />
                </td>
                {/* Name */}
                <td style={{ padding: '11px 12px' }}>
                  <div style={{
                    fontSize: 13,
                    fontWeight: 500,
                    color: 'var(--text-primary)',
                    marginBottom: 2,
                  }}>
                    {row.name || 'Unnamed'}
                  </div>
                  <div style={{
                    fontSize: 10,
                    color: 'var(--text-muted)',
                    fontFamily: 'monospace',
                    letterSpacing: '0.02em',
                  }}>
                    {row.id.slice(0, 12)}
                  </div>
                </td>
                {/* Tags / job type */}
                <td style={{ padding: '11px 12px' }}>
                  <span style={{
                    fontSize: 11,
                    padding: '2px 7px',
                    borderRadius: 4,
                    background: 'var(--bg-active)',
                    color: 'var(--text-secondary)',
                    border: '1px solid var(--border)',
                    whiteSpace: 'nowrap',
                  }}>
                    {row.jobType === 'training_run' ? 'Training' : row.jobType === 'rl_env' ? 'RL Env' : 'FaaS'}
                  </span>
                </td>
                {/* Hardware */}
                <td style={{ padding: '11px 12px' }}>
                  <span style={{
                    fontSize: 11,
                    padding: '2px 7px',
                    borderRadius: 4,
                    background: row.hardwareType === 'gpu' ? 'var(--accent-dim)' : 'rgba(107,107,107,0.08)',
                    color: row.hardwareType === 'gpu' ? 'var(--accent)' : 'var(--text-secondary)',
                    border: `1px solid ${row.hardwareType === 'gpu' ? 'var(--accent-border)' : 'var(--border)'}`,
                    fontWeight: 500,
                    whiteSpace: 'nowrap',
                  }}>
                    {row.hardwareType === 'gpu' ? `GPU · ${row.gpuModel || 'a100'}` : 'CPU'}
                  </span>
                </td>
                {/* Status */}
                <td style={{ padding: '11px 12px' }}>
                  <span style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 5,
                    fontSize: 11,
                    padding: '2px 8px',
                    borderRadius: 5,
                    background: sc.bg,
                    color: sc.label,
                    border: `1px solid ${sc.border}`,
                    fontWeight: 500,
                    whiteSpace: 'nowrap',
                  }}>
                    <span style={{
                      width: 5, height: 5, borderRadius: '50%',
                      background: sc.dot,
                      display: 'inline-block',
                      animation: isRunning ? 'pulse 1.8s ease-in-out infinite' : 'none',
                    }} />
                    {row.status}
                  </span>
                </td>
                {/* Progress */}
                <td style={{ padding: '11px 12px', minWidth: 140 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <div style={{
                      flex: 1,
                      height: 3,
                      background: 'var(--border-light)',
                      borderRadius: 2,
                      overflow: 'hidden',
                    }}>
                      <div style={{
                        height: '100%',
                        width: `${row.progress}%`,
                        background: isRunning ? 'var(--running)' : row.status === 'completed' ? 'var(--success)' : 'var(--text-muted)',
                        borderRadius: 2,
                        transition: 'width 0.6s ease',
                      }} />
                    </div>
                    <span style={{
                      fontSize: 11,
                      color: 'var(--text-secondary)',
                      width: 30,
                      textAlign: 'right',
                      fontVariantNumeric: 'tabular-nums',
                    }}>
                      {row.progress}%
                    </span>
                  </div>
                </td>
                {/* Created */}
                <td style={{ padding: '11px 12px', whiteSpace: 'nowrap' }}>
                  <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{fmtDate(row.createdAt)}</span>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function TableHeader() {
  return (
    <table style={{ width: '100%', borderCollapse: 'collapse' }}>
      <thead>
        <tr style={{ borderBottom: '1px solid var(--border)' }}>
          <th style={{ width: 36, padding: '9px 8px 9px 16px' }}>
            <div style={{ width: 14, height: 14, border: '1px solid var(--border-light)', borderRadius: 3, background: 'var(--surface)' }} />
          </th>
          {COLS.map(col => (
            <th key={col.label} style={{
              padding: '9px 12px',
              textAlign: 'left',
              fontSize: 11,
              fontWeight: 500,
              color: 'var(--text-secondary)',
              letterSpacing: '0.02em',
              whiteSpace: 'nowrap',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                {col.label}
                <SortIcon />
              </div>
            </th>
          ))}
        </tr>
      </thead>
    </table>
  )
}

function SortIcon() {
  return (
    <svg width="8" height="10" viewBox="0 0 8 10" fill="none" style={{ opacity: 0.3 }}>
      <path d="M4 1L4 9M1.5 3.5L4 1L6.5 3.5M1.5 6.5L4 9L6.5 6.5" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  )
}
