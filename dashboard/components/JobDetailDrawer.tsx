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
    case 'running':   return { fg: '#3b82f6', bg: 'rgba(59,130,246,0.1)' }
    case 'completed': return { fg: '#22c55e', bg: 'rgba(34,197,94,0.1)' }
    case 'error':
    case 'failed':    return { fg: '#ef4444', bg: 'rgba(239,68,68,0.1)' }
    default:          return { fg: '#6b6b6b', bg: 'rgba(107,107,107,0.1)' }
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

  useEffect(() => {
    if (!executionId) { setExec(null); return }
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
        // stop polling once terminal
        if (d.Status === 'completed' || d.Status === 'failed' || d.Status === 'error') {
          clearInterval(interval)
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
        width: 420,
        background: '#0e0e0e',
        borderLeft: '1px solid #252525',
        zIndex: 50,
        display: 'flex',
        flexDirection: 'column',
        transform: visible ? 'translateX(0)' : 'translateX(100%)',
        transition: 'transform 0.2s cubic-bezier(0.16,1,0.3,1)',
      }}>
        {/* Header */}
        <div style={{
          height: 52,
          borderBottom: '1px solid #1f1f1f',
          display: 'flex', alignItems: 'center',
          padding: '0 20px', gap: 10, flexShrink: 0,
        }}>
          <span style={{ fontFamily: 'var(--font-grotesk), sans-serif', fontSize: 14, fontWeight: 600, color: '#e8e8e8', flex: 1 }}>
            Run Detail
          </span>
          <button
            onClick={onClose}
            style={{ background: 'none', border: 'none', color: '#6b6b6b', cursor: 'pointer', padding: 4, lineHeight: 1 }}
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
                        fontSize: 12, fontWeight: 500, padding: '3px 10px', borderRadius: 5,
                        background: sc.bg, color: sc.fg,
                        display: 'inline-flex', alignItems: 'center', gap: 5,
                      }}>
                        <span style={{ width: 5, height: 5, borderRadius: '50%', background: sc.fg, display: 'inline-block' }} />
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

              {/* Success check guide */}
              <Section title="How to verify">
                <div style={{ fontSize: 12, color: '#6b6b6b', lineHeight: 1.7 }}>
                  {(() => {
                    const isPending = exec.Status === 'pending'
                    const isRunning = exec.Status === 'running'
                    const isDone = exec.Status === 'completed'
                    const isFailed = exec.Status === 'failed' || exec.Status === 'error'
                    const hasDseq = !!(exec.HardwareType === 'gpu' && exec.VMID)
                    return (
                      <>
                        <CheckItem ok={hasDseq} pending={!hasDseq && isPending} text="Deployment created on Akash (dseq present)" />
                        <CheckItem ok={hasDseq} pending={!hasDseq && isPending} text="Bid received and lease created" />
                        <CheckItem
                          ok={isDone}
                          pending={isPending || isRunning}
                          text={isPending ? 'Waiting for container to start…' : isRunning ? 'Training in progress…' : isDone ? 'Execution completed successfully' : `Execution failed: ${exec.Error || 'see error above'}`}
                        />
                        {isFailed && exec.Error && exec.Error.includes('forwardedPorts stayed null') && (
                          <div style={{ marginTop: 8, padding: '8px 10px', background: 'rgba(245,158,11,0.08)', border: '1px solid rgba(245,158,11,0.2)', borderRadius: 6 }}>
                            <span style={{ fontSize: 11, color: '#f59e0b' }}>
                              Container did not expose port 8081. It may have crashed on startup or the image is still pulling. Check Akash Console for provider logs.
                            </span>
                          </div>
                        )}
                        {isFailed && exec.Error && !exec.Error.includes('forwardedPorts') && !exec.Error.includes('allocate VM') && (
                          <div style={{ marginTop: 8, padding: '8px 10px', background: 'rgba(245,158,11,0.08)', border: '1px solid rgba(245,158,11,0.2)', borderRadius: 6 }}>
                            <span style={{ fontSize: 11, color: '#f59e0b' }}>
                              GPU was provisioned correctly. Error is in the container workload.
                            </span>
                          </div>
                        )}
                      </>
                    )
                  })()}
                </div>
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
    <div>
      <div style={{
        fontSize: 10, fontWeight: 600, color: '#3d3d3d',
        letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: 10,
      }}>
        {title}
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {children}
      </div>
    </div>
  )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
      <span style={{ fontSize: 11, color: '#6b6b6b', minWidth: 120, flexShrink: 0 }}>{label}</span>
      <span style={{ fontSize: 12, color: '#e8e8e8', fontFamily: mono ? 'monospace' : 'inherit', wordBreak: 'break-all' }}>
        {value || '—'}
      </span>
    </div>
  )
}

function CheckItem({ ok, pending, text }: { ok: boolean; pending?: boolean; text: string }) {
  const color = ok ? '#22c55e' : pending ? '#6b6b6b' : '#ef4444'
  const icon = ok ? '✓' : pending ? '·' : '✗'
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 7, marginBottom: 4 }}>
      <span style={{ color, fontSize: 12, marginTop: 1, flexShrink: 0 }}>{icon}</span>
      <span style={{ fontSize: 12, color: ok ? '#a0a0a0' : pending ? '#5a5a5a' : '#ef4444' }}>{text}</span>
    </div>
  )
}
