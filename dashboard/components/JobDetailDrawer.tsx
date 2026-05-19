'use client'

import { useEffect, useState } from 'react'
import { Execution } from '@/types'
import { api } from '@/lib/api'

interface Props {
  executionId: string | null
  onClose: () => void
}

function statusColor(s: string) {
  switch (s) {
    case 'running':   return { fg: 'var(--running)',  bg: 'var(--running-dim)',  border: 'rgba(59,130,246,0.2)' }
    case 'queued':    return { fg: 'var(--queued)',   bg: 'rgba(163,163,163,0.08)', border: 'rgba(163,163,163,0.15)' }
    case 'completed': return { fg: 'var(--success)',  bg: 'var(--success-dim)', border: 'rgba(34,197,94,0.2)' }
    case 'error':
    case 'failed':    return { fg: 'var(--error)',    bg: 'var(--error-dim)',   border: 'rgba(239,68,68,0.2)' }
    default:          return { fg: 'var(--text-secondary)', bg: 'rgba(107,107,107,0.08)', border: 'var(--border)' }
  }
}

function fmtDuration(ms: number) {
  if (!ms) return '—'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

function fmtTime(s: string) {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

export function JobDetailDrawer({ executionId, onClose }: Props) {
  const [exec, setExec] = useState<Execution | null>(null)
  const [loading, setLoading] = useState(false)
  const [artifacts, setArtifacts] = useState<string[]>([])

  useEffect(() => {
    if (!executionId) return
    let cancelled = false
    const poll = () => {
      if (cancelled) return
      setLoading(prev => prev === false ? false : true) // only show spinner on first load
      api.getExecution(executionId)
        .then(d => { if (!cancelled) { setExec(d); setLoading(false) } })
        .catch(() => { if (!cancelled) { setExec(null); setLoading(false) } })
    }
    poll()
    const interval = setInterval(() => {
      if (cancelled) return
      api.getExecution(executionId).then(d => {
        if (!cancelled) setExec(d)
        if (d.Status === 'completed' || d.Status === 'failed' || d.Status === 'error') {
          clearInterval(interval)
          // Fetch artifacts once the job is terminal.
          api.listArtifacts(executionId).then(a => { if (!cancelled) setArtifacts(a) }).catch(() => {})
        }
      }).catch(() => {})
    }, 4000)
    return () => { cancelled = true; clearInterval(interval) }
  }, [executionId])

  const visible = !!executionId

  return (
    <>
      {/* Backdrop */}
      {visible && (
        <div
          onClick={onClose}
          style={{
            position: 'fixed', inset: 0,
            background: 'rgba(0,0,0,0.4)',
            zIndex: 40,
          }}
        />
      )}

      {/* Drawer */}
      <div style={{
        position: 'fixed',
        top: 0, right: 0, bottom: 0,
        width: 440,
        background: 'var(--bg-panel)',
        borderLeft: '1px solid var(--border-light)',
        zIndex: 50,
        display: 'flex',
        flexDirection: 'column',
        transform: visible ? 'translateX(0)' : 'translateX(100%)',
        transition: 'transform 0.22s cubic-bezier(0.16,1,0.3,1)',
        boxShadow: '-16px 0 48px rgba(0,0,0,0.5)',
      }}>
        {/* Header */}
        <div style={{
          height: 54,
          borderBottom: '1px solid var(--border)',
          display: 'flex', alignItems: 'center',
          padding: '0 20px', gap: 10, flexShrink: 0,
        }}>
          <span style={{
            fontFamily: 'var(--font-grotesk), sans-serif',
            fontSize: 14,
            fontWeight: 600,
            color: 'var(--text-primary)',
            flex: 1,
            letterSpacing: '-0.01em',
          }}>
            Run Detail
          </span>
          <button
            onClick={onClose}
            style={{
              background: 'var(--bg-active)',
              border: '1px solid var(--border)',
              borderRadius: 6,
              color: 'var(--text-secondary)',
              cursor: 'pointer',
              padding: '5px 6px',
              lineHeight: 1,
              display: 'flex',
              alignItems: 'center',
              transition: 'background 0.1s',
            }}
            onMouseEnter={e => (e.currentTarget as HTMLButtonElement).style.background = 'var(--bg-hover)'}
            onMouseLeave={e => (e.currentTarget as HTMLButtonElement).style.background = 'var(--bg-active)'}
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
              <path d="M1 1l12 12M13 1L1 13"/>
            </svg>
          </button>
        </div>

        {/* Body */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
          {loading && (
            <div style={{ color: '#6b6b6b', fontSize: 13 }}>Loading…</div>
          )}

          {!loading && !exec && executionId && (
            <div style={{ color: '#6b6b6b', fontSize: 13 }}>Execution not found.</div>
          )}

          {!loading && exec && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
              {/* Status + ID */}
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                  {(() => {
                    const sc = statusColor(exec.Status)
                    return (
                      <span style={{
                        fontSize: 11,
                        fontWeight: 500,
                        padding: '3px 10px',
                        borderRadius: 5,
                        background: sc.bg,
                        color: sc.fg,
                        border: `1px solid ${sc.border}`,
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: 5,
                      }}>
                        <span style={{
                          width: 5, height: 5, borderRadius: '50%',
                          background: sc.fg, display: 'inline-block',
                          animation: exec.Status === 'running' ? 'pulse 1.8s ease-in-out infinite' : 'none',
                        }} />
                        {exec.Status}
                      </span>
                    )
                  })()}
                  <span style={{ fontSize: 11, color: '#3d3d3d', fontFamily: 'monospace' }}>
                    {exec.ID}
                  </span>
                </div>
              </div>

              {/* Key-value grid */}
              <Section title="Execution">
                <Row label="Function" value={exec.FunctionName || exec.FunctionID} mono />
                <Row label="Job Type" value={exec.JobType || 'faas_function'} />
                <Row label="Hardware" value={exec.HardwareType === 'gpu' ? `GPU · ${exec.GPUModel}` : 'CPU (Firecracker)'} />
                <Row label="Duration" value={fmtDuration(exec.Duration)} />
                <Row label="Started" value={fmtTime(exec.StartTime)} />
                <Row label="Ended" value={fmtTime(exec.EndTime)} />
              </Section>

              {exec.HardwareType === 'gpu' && exec.VMID && (
                <Section title="Akash Deployment">
                  <Row label="Deployment (dseq)" value={exec.VMID} mono />
                  <Row label="Status" value={exec.Status === 'error' ? 'Closed' : exec.Status === 'completed' ? 'Closed' : 'Active'} />
                  {/^\d+$/.test(exec.VMID) && (
                    <div style={{ marginTop: 8 }}>
                      <a
                        href={`https://console.akash.network/deployments/${exec.VMID}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        style={{
                          fontSize: 12, color: 'var(--accent)',
                          textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: 4,
                        }}
                      >
                        View on Akash Console
                        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                          <path d="M1 9L9 1M9 1H4M9 1v5"/>
                        </svg>
                      </a>
                    </div>
                  )}
                </Section>
              )}

              {exec.Error && (
                <Section title="Error">
                  <pre style={{
                    fontSize: 11, color: '#ef4444', background: 'rgba(239,68,68,0.06)',
                    border: '1px solid rgba(239,68,68,0.15)', borderRadius: 6,
                    padding: '10px 12px', whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                    fontFamily: 'monospace', lineHeight: 1.6, margin: 0,
                  }}>
                    {exec.Error}
                  </pre>
                </Section>
              )}

              {exec.Logs ? (
                <Section title="Logs">
                  <pre style={{
                    fontSize: 11, color: '#a0a0a0', background: '#0a0a0a',
                    border: '1px solid #1f1f1f', borderRadius: 6,
                    padding: '10px 12px', whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                    fontFamily: 'monospace', lineHeight: 1.6, margin: 0, maxHeight: 300, overflowY: 'auto',
                  }}>
                    {exec.Logs}
                  </pre>
                </Section>
              ) : (
                <Section title="Logs">
                  <span style={{ fontSize: 12, color: '#3d3d3d' }}>No logs captured.</span>
                </Section>
              )}

              {/* Artifacts */}
              <Section title="Artifacts">
                {artifacts.length === 0 ? (
                  <span style={{ fontSize: 12, color: '#3d3d3d' }}>No artifacts.</span>
                ) : (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
                    {artifacts.map(name => (
                      <a
                        key={name}
                        href={api.artifactDownloadURL(executionId!, name)}
                        target="_blank"
                        rel="noopener noreferrer"
                        style={{ fontSize: 12, color: 'var(--accent)', textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: 4 }}
                      >
                        <svg width="11" height="11" viewBox="0 0 11 11" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                          <path d="M5.5 1v6M2.5 5l3 3 3-3M1 10h9"/>
                        </svg>
                        {name}
                      </a>
                    ))}
                  </div>
                )}
              </Section>
            </div>
          )}
        </div>
      </div>
    </>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{
      background: 'var(--surface)',
      border: '1px solid var(--border)',
      borderRadius: 8,
      overflow: 'hidden',
    }}>
      <div style={{
        padding: '8px 14px',
        borderBottom: '1px solid var(--border)',
        fontSize: 10,
        fontWeight: 600,
        color: 'var(--text-secondary)',
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
      }}>
        {title}
      </div>
      <div style={{ padding: '10px 14px', display: 'flex', flexDirection: 'column', gap: 8 }}>
        {children}
      </div>
    </div>
  )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
      <span style={{ fontSize: 11, color: 'var(--text-secondary)', minWidth: 110, flexShrink: 0 }}>{label}</span>
      <span style={{
        fontSize: 12,
        color: 'var(--text-primary)',
        fontFamily: mono ? 'monospace' : 'inherit',
        wordBreak: 'break-all',
      }}>
        {value || '—'}
      </span>
    </div>
  )
}

