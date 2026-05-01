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

function statusColor(s: string) {
  switch (s) {
    case 'running': return { dot: '#3b82f6', label: '#3b82f6', bg: 'rgba(59,130,246,0.1)' }
    case 'completed': return { dot: '#22c55e', label: '#22c55e', bg: 'rgba(34,197,94,0.1)' }
    case 'failed': return { dot: '#ef4444', label: '#ef4444', bg: 'rgba(239,68,68,0.1)' }
    default: return { dot: '#6b6b6b', label: '#6b6b6b', bg: 'rgba(107,107,107,0.1)' }
  }
}

function fmtDate(s: string) {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d.getTime())) return '—'
  const now = Date.now()
  const diff = now - d.getTime()
  if (diff < 60000) return 'just now'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

function getProgress(jobID: string, metrics: Record<string, import('@/types').TrainingMetric[]>): number {
  const m = metrics[jobID]
  if (!m || m.length === 0) return 0
  const last = m[m.length - 1]
  // assume 50000 total steps
  return Math.min(100, Math.round((last.step / 50000) * 100))
}

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
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 16, padding: 48 }}>
        <div style={{
          width: 48, height: 48,
          border: '1px solid var(--border-light)',
          borderRadius: 10,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="var(--text-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="1,15 7,8 11,11 19,4" />
          </svg>
        </div>
        <div style={{ textAlign: 'center' }}>
          <div style={{ fontSize: 14, fontWeight: 500, color: 'var(--text-primary)', marginBottom: 4 }}>
            No training runs yet
          </div>
          <div style={{ fontSize: 13, color: 'var(--text-secondary)', maxWidth: 280, lineHeight: 1.5 }}>
            Submit a job to start training a model on GPU compute.
          </div>
        </div>
        <button
          onClick={onNewRun}
          style={{
            padding: '8px 16px',
            background: 'var(--accent)',
            color: 'white',
            border: 'none',
            borderRadius: 7,
            fontSize: 13,
            fontWeight: 500,
            cursor: 'pointer',
            fontFamily: 'inherit',
          }}
        >
          New Run
        </button>
      </div>
    )
  }

  return (
    <div style={{ flex: 1, overflowY: 'auto' }}>
      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr style={{ borderBottom: '1px solid var(--border)' }}>
            {['Name', 'Type', 'Hardware', 'Status', 'Progress', 'Created', 'Updated'].map(col => (
              <th key={col} style={{
                padding: '8px 16px',
                textAlign: 'left',
                fontSize: 11,
                fontWeight: 500,
                color: 'var(--text-secondary)',
                letterSpacing: '0.04em',
                whiteSpace: 'nowrap',
              }}>
                {col}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map(row => {
            const sc = statusColor(row.status)
            return (
              <tr
                key={row.id}
                onClick={() => onSelectExecution(row.id)}
                style={{ borderBottom: '1px solid var(--border)', cursor: 'pointer', transition: 'background 0.1s' }}
                onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'}
                onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'transparent'}
              >
                <td style={{ padding: '10px 16px' }}>
                  <div style={{ fontSize: 13, fontWeight: 500, color: 'var(--text-primary)' }}>{row.name}</div>
                  <div style={{ fontSize: 11, color: 'var(--text-secondary)', marginTop: 1, fontFamily: 'monospace' }}>
                    {row.id.slice(0, 8)}
                  </div>
                </td>
                <td style={{ padding: '10px 16px' }}>
                  <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                    {row.jobType === 'training_run' ? 'Training' : row.jobType === 'rl_env' ? 'RL Env' : 'FaaS'}
                  </span>
                </td>
                <td style={{ padding: '10px 16px' }}>
                  <span style={{
                    fontSize: 11,
                    padding: '2px 7px',
                    borderRadius: 4,
                    background: row.hardwareType === 'gpu' ? 'rgba(124,92,252,0.12)' : 'rgba(107,107,107,0.1)',
                    color: row.hardwareType === 'gpu' ? 'var(--accent)' : 'var(--text-secondary)',
                    fontWeight: 500,
                  }}>
                    {row.hardwareType === 'gpu' ? `GPU · ${row.gpuModel || 'rtx3090'}` : 'CPU'}
                  </span>
                </td>
                <td style={{ padding: '10px 16px' }}>
                  <span style={{
                    display: 'inline-flex', alignItems: 'center', gap: 5,
                    fontSize: 12, padding: '2px 8px', borderRadius: 4,
                    background: sc.bg, color: sc.label,
                  }}>
                    <span style={{ width: 5, height: 5, borderRadius: '50%', background: sc.dot, display: 'inline-block' }} />
                    {row.status}
                  </span>
                </td>
                <td style={{ padding: '10px 16px', minWidth: 120 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <div style={{
                      flex: 1, height: 3, background: 'var(--border-light)', borderRadius: 2, overflow: 'hidden',
                    }}>
                      <div style={{
                        height: '100%',
                        width: `${row.progress}%`,
                        background: row.status === 'running' ? 'var(--running)' : row.status === 'completed' ? 'var(--success)' : 'var(--text-muted)',
                        borderRadius: 2,
                        transition: 'width 0.5s ease',
                      }} />
                    </div>
                    <span style={{ fontSize: 11, color: 'var(--text-secondary)', width: 28, textAlign: 'right' }}>
                      {row.progress}%
                    </span>
                  </div>
                </td>
                <td style={{ padding: '10px 16px', whiteSpace: 'nowrap' }}>
                  <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{fmtDate(row.createdAt)}</span>
                </td>
                <td style={{ padding: '10px 16px', whiteSpace: 'nowrap' }}>
                  <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{fmtDate(row.updatedAt)}</span>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
