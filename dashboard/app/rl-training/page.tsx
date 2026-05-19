'use client'

import { useEffect, useState, useCallback } from 'react'
import { api, RLRun, RLRunDetail } from '@/lib/api'
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from 'recharts'

// ── Types ────────────────────────────────────────────────────────────────────

interface NewRunForm {
  base_model: string
  num_workers: number
  gpu_model: string
}

const defaultForm: NewRunForm = {
  base_model: 'Qwen/Qwen3-0.6B',
  num_workers: 2,
  gpu_model: 'a100',
}

const GPU_MODELS = ['a100', 'h100', 't4', 'l4', 'a6000']
const BASE_MODELS = [
  'Qwen/Qwen3-0.6B',                                    // 0.6B — official RL post-trained
  'rasdani/Qwen2.5-0.5B-Open-R1-Code-GRPO',             // 0.5B — GRPO on coding problems
  'Qwen/Qwen2.5-Coder-1.5B-Instruct',
  'Qwen/Qwen2.5-Coder-7B-Instruct',
  'deepseek-ai/DeepSeek-Coder-1.3B-Instruct',
]

// ── Status helpers ────────────────────────────────────────────────────────────

function statusColor(s: string) {
  switch (s) {
    case 'running':   return { fg: 'var(--running)',  bg: 'var(--running-dim)',  border: 'rgba(59,130,246,0.2)' }
    case 'starting':  return { fg: '#f59e0b',          bg: 'rgba(245,158,11,0.08)', border: 'rgba(245,158,11,0.2)' }
    case 'completed': return { fg: 'var(--success)',   bg: 'var(--success-dim)', border: 'rgba(34,197,94,0.2)' }
    case 'stopped':   return { fg: 'var(--text-secondary)', bg: 'rgba(107,107,107,0.08)', border: 'var(--border)' }
    default:          return { fg: 'var(--error)',     bg: 'var(--error-dim)',   border: 'rgba(239,68,68,0.2)' }
  }
}

function StatusBadge({ status }: { status: string }) {
  const sc = statusColor(status)
  return (
    <span style={{
      fontSize: 11, fontWeight: 500, padding: '3px 10px', borderRadius: 5,
      background: sc.bg, color: sc.fg, border: `1px solid ${sc.border}`,
      display: 'inline-flex', alignItems: 'center', gap: 5,
    }}>
      <span style={{
        width: 5, height: 5, borderRadius: '50%', background: sc.fg,
        animation: status === 'running' || status === 'starting' ? 'pulse 1.8s ease-in-out infinite' : 'none',
      }} />
      {status}
    </span>
  )
}

// ── New Run Dialog ────────────────────────────────────────────────────────────

function NewRunDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (runId: string) => void }) {
  const [form, setForm] = useState<NewRunForm>(defaultForm)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
    setLoading(true)
    setError('')
    try {
      const result = await api.startRLRun(form)
      onCreated(result.run_id)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to start run')
      setLoading(false)
    }
  }

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', zIndex: 40 }} />
      <div style={{
        position: 'fixed', top: '50%', left: '50%',
        transform: 'translate(-50%, -50%)',
        background: 'var(--bg-panel)', border: '1px solid var(--border)',
        borderRadius: 12, width: 480, zIndex: 50,
        boxShadow: '0 24px 64px rgba(0,0,0,0.6)',
      }}>
        {/* Header */}
        <div style={{ padding: '18px 20px', borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontFamily: 'var(--font-grotesk)', fontSize: 14, fontWeight: 600, color: 'var(--text-primary)' }}>
            New RL Training Run
          </span>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-secondary)', padding: 4 }}>
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
              <path d="M1 1l12 12M13 1L1 13"/>
            </svg>
          </button>
        </div>

        {/* Body */}
        <div style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 16 }}>
          <Field label="Base Model">
            <select
              value={form.base_model}
              onChange={e => setForm(f => ({ ...f, base_model: e.target.value }))}
              style={selectStyle}
            >
              {BASE_MODELS.map(m => <option key={m} value={m}>{m}</option>)}
            </select>
          </Field>

          <Field label="GPU Model">
            <select
              value={form.gpu_model}
              onChange={e => setForm(f => ({ ...f, gpu_model: e.target.value }))}
              style={selectStyle}
            >
              {GPU_MODELS.map(m => <option key={m} value={m}>{m.toUpperCase()}</option>)}
            </select>
          </Field>

          <Field label="Rollout Workers">
            <input
              type="number" min={1} max={16}
              value={form.num_workers}
              onChange={e => setForm(f => ({ ...f, num_workers: Number(e.target.value) }))}
              style={inputStyle}
            />
          </Field>

          {error && (
            <div style={{ fontSize: 12, color: 'var(--error)', background: 'var(--error-dim)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 6, padding: '8px 12px' }}>
              {error}
            </div>
          )}
        </div>

        {/* Footer */}
        <div style={{ padding: '12px 20px', borderTop: '1px solid var(--border)', display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button onClick={onClose} style={secondaryBtn}>Cancel</button>
          <button onClick={submit} disabled={loading} style={primaryBtn}>
            {loading ? 'Starting…' : 'Start Run'}
          </button>
        </div>
      </div>
    </>
  )
}

// ── Run Detail Panel ──────────────────────────────────────────────────────────

function RunDetailPanel({ runId, onClose }: { runId: string; onClose: () => void }) {
  const [detail, setDetail] = useState<RLRunDetail | null>(null)

  const refresh = useCallback(async () => {
    try {
      const d = await api.getRLRun(runId)
      setDetail(d)
    } catch {}
  }, [runId])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [refresh])

  const stop = async () => {
    await api.stopRLRun(runId)
    refresh()
  }

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', zIndex: 40 }} />
      <div style={{
        position: 'fixed', top: 0, right: 0, bottom: 0, width: 520,
        background: 'var(--bg-panel)', borderLeft: '1px solid var(--border-light)',
        zIndex: 50, display: 'flex', flexDirection: 'column',
        boxShadow: '-16px 0 48px rgba(0,0,0,0.5)',
      }}>
        {/* Header */}
        <div style={{ height: 54, borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', padding: '0 20px', gap: 10, flexShrink: 0 }}>
          <span style={{ fontFamily: 'var(--font-grotesk)', fontSize: 14, fontWeight: 600, color: 'var(--text-primary)', flex: 1 }}>
            Run Detail
          </span>
          {detail && (detail.run.Status === 'running' || detail.run.Status === 'starting') && (
            <button onClick={stop} style={{ ...secondaryBtn, fontSize: 11, padding: '4px 10px', color: 'var(--error)' }}>
              Stop
            </button>
          )}
          <button onClick={onClose} style={{ background: 'var(--bg-active)', border: '1px solid var(--border)', borderRadius: 6, color: 'var(--text-secondary)', cursor: 'pointer', padding: '5px 6px', display: 'flex', alignItems: 'center' }}>
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
              <path d="M1 1l12 12M13 1L1 13"/>
            </svg>
          </button>
        </div>

        {/* Body */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 20, display: 'flex', flexDirection: 'column', gap: 16 }}>
          {!detail && <div style={{ color: 'var(--text-secondary)', fontSize: 13 }}>Loading…</div>}
          {detail && <>
            {/* Meta */}
            <Section title="Run Info">
              <KV label="Run ID" value={detail.run.ID} mono />
              <KV label="Status" value={<StatusBadge status={detail.run.Status} />} />
              <KV label="Base Model" value={detail.run.BaseModel} mono />
              <KV label="GPU Model" value={detail.run.GPUModel.toUpperCase()} />
              <KV label="Workers" value={String(detail.run.NumWorkers)} />
              <KV label="Buffer" value={`${detail.buffer_size} trajectories`} />
            </Section>

            {/* Worker statuses */}
            <Section title="Workers">
              {detail.worker_statuses.length === 0
                ? <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>No workers yet.</span>
                : detail.worker_statuses.map((w, i) => (
                  <div key={w.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <span style={{ fontSize: 11, color: 'var(--text-secondary)', fontFamily: 'monospace' }}>worker-{i}</span>
                    <StatusBadge status={w.status} />
                  </div>
                ))
              }
              {detail.trainer_status && (
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 4, paddingTop: 8, borderTop: '1px solid var(--border)' }}>
                  <span style={{ fontSize: 11, color: 'var(--text-secondary)' }}>trainer</span>
                  <StatusBadge status={detail.trainer_status} />
                </div>
              )}
            </Section>

            {/* Charts */}
            {detail.metrics && detail.metrics.length > 0 && (
              <Section title="Reward Curve">
                <div style={{ height: 160 }}>
                  <ResponsiveContainer width="100%" height="100%">
                    <LineChart data={detail.metrics} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
                      <XAxis dataKey="step" tick={{ fontSize: 10, fill: 'var(--text-secondary)' }} />
                      <YAxis tick={{ fontSize: 10, fill: 'var(--text-secondary)' }} domain={[0, 1]} />
                      <Tooltip contentStyle={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 6, fontSize: 11 }} />
                      <Legend wrapperStyle={{ fontSize: 11 }} />
                      <Line type="monotone" dataKey="episode_reward" stroke="var(--accent)" dot={false} strokeWidth={1.5} name="Reward" />
                      <Line type="monotone" dataKey="loss" stroke="var(--error)" dot={false} strokeWidth={1.5} name="Loss" />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              </Section>
            )}
            {(!detail.metrics || detail.metrics.length === 0) && (
              <Section title="Reward Curve">
                <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>No metrics yet — workers are collecting trajectories.</span>
              </Section>
            )}
          </>}
        </div>
      </div>
    </>
  )
}

// ── Main Page ─────────────────────────────────────────────────────────────────

export default function RLTrainingPage() {
  const [runs, setRuns] = useState<RLRun[]>([])
  const [showNewRun, setShowNewRun] = useState(false)
  const [selectedRun, setSelectedRun] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      const data = await api.listRLRuns()
      setRuns(Array.isArray(data) ? data : [])
    } catch {}
    setLoading(false)
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 8000)
    return () => clearInterval(t)
  }, [refresh])

  const handleCreated = (runId: string) => {
    setShowNewRun(false)
    setSelectedRun(runId)
    refresh()
  }

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
      {/* Top bar */}
      <div style={{
        height: 56, borderBottom: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', padding: '0 24px',
        gap: 12, flexShrink: 0, background: 'var(--bg-panel)',
      }}>
        <span style={{ fontFamily: 'var(--font-grotesk)', fontSize: 15, fontWeight: 600, color: 'var(--text-primary)', flex: 1, letterSpacing: '-0.02em' }}>
          RL Training
        </span>
        <button onClick={() => setShowNewRun(true)} style={primaryBtn}>
          + New Run
        </button>
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflowY: 'auto', padding: 24 }}>
        {loading && <div style={{ color: 'var(--text-secondary)', fontSize: 13 }}>Loading…</div>}

        {!loading && runs.length === 0 && (
          <div style={{
            display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
            padding: 80, gap: 16, textAlign: 'center',
          }}>
            <div style={{ width: 48, height: 48, borderRadius: 12, background: 'var(--surface)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <svg width="22" height="22" viewBox="0 0 14 14" fill="none" stroke="var(--text-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="7" cy="7" r="2.5"/><path d="M7 1v2M7 11v2M1 7h2M11 7h2"/>
              </svg>
            </div>
            <div>
              <div style={{ fontFamily: 'var(--font-grotesk)', fontSize: 15, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 6 }}>
                No RL runs yet
              </div>
              <div style={{ fontSize: 13, color: 'var(--text-secondary)', maxWidth: 300, lineHeight: 1.6 }}>
                Start a distributed RL training run to fine-tune a coding agent using GRPO.
              </div>
            </div>
            <button onClick={() => setShowNewRun(true)} style={primaryBtn}>
              Start your first run
            </button>
          </div>
        )}

        {!loading && runs.length > 0 && (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Run ID', 'Model', 'GPU', 'Workers', 'Status', 'Created'].map(col => (
                  <th key={col} style={{
                    padding: '8px 12px', textAlign: 'left', fontSize: 11,
                    fontWeight: 600, color: 'var(--text-secondary)',
                    borderBottom: '1px solid var(--border)',
                    letterSpacing: '0.04em', textTransform: 'uppercase',
                  }}>{col}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {runs.map(run => (
                <tr
                  key={run.ID}
                  onClick={() => setSelectedRun(run.ID)}
                  style={{ cursor: 'pointer', transition: 'background 0.1s' }}
                  onMouseEnter={e => (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)'}
                  onMouseLeave={e => (e.currentTarget as HTMLElement).style.background = 'transparent'}
                >
                  <td style={tdStyle}>
                    <span style={{ fontFamily: 'monospace', fontSize: 12, color: 'var(--accent)' }}>{run.ID}</span>
                  </td>
                  <td style={tdStyle}>
                    <span style={{ fontSize: 12, color: 'var(--text-primary)', fontFamily: 'monospace' }}>
                      {run.BaseModel.split('/').pop()}
                    </span>
                  </td>
                  <td style={tdStyle}>
                    <span style={{ fontSize: 11, padding: '2px 8px', borderRadius: 4, background: 'rgba(124,92,252,0.1)', color: 'var(--accent)', border: '1px solid var(--accent-border)' }}>
                      {run.GPUModel.toUpperCase()}
                    </span>
                  </td>
                  <td style={tdStyle}>
                    <span style={{ fontSize: 12, color: 'var(--text-primary)' }}>{run.NumWorkers}</span>
                  </td>
                  <td style={tdStyle}>
                    <StatusBadge status={run.Status} />
                  </td>
                  <td style={tdStyle}>
                    <span style={{ fontSize: 11, color: 'var(--text-secondary)' }}>
                      {new Date(run.CreatedAt).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showNewRun && <NewRunDialog onClose={() => setShowNewRun(false)} onCreated={handleCreated} />}
      {selectedRun && <RunDetailPanel runId={selectedRun} onClose={() => setSelectedRun(null)} />}
    </div>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8, overflow: 'hidden' }}>
      <div style={{ padding: '8px 14px', borderBottom: '1px solid var(--border)', fontSize: 10, fontWeight: 600, color: 'var(--text-secondary)', letterSpacing: '0.08em', textTransform: 'uppercase' }}>
        {title}
      </div>
      <div style={{ padding: '10px 14px', display: 'flex', flexDirection: 'column', gap: 8 }}>
        {children}
      </div>
    </div>
  )
}

function KV({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
      <span style={{ fontSize: 11, color: 'var(--text-secondary)', minWidth: 100, flexShrink: 0 }}>{label}</span>
      {typeof value === 'string'
        ? <span style={{ fontSize: 12, color: 'var(--text-primary)', fontFamily: mono ? 'monospace' : 'inherit', wordBreak: 'break-all' }}>{value || '—'}</span>
        : value
      }
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <label style={{ fontSize: 11, fontWeight: 500, color: 'var(--text-secondary)', letterSpacing: '0.04em', textTransform: 'uppercase' }}>{label}</label>
      {children}
    </div>
  )
}

const tdStyle: React.CSSProperties = {
  padding: '10px 12px',
  borderBottom: '1px solid var(--border)',
  verticalAlign: 'middle',
}

const selectStyle: React.CSSProperties = {
  width: '100%', padding: '8px 10px', fontSize: 13,
  background: 'var(--surface)', border: '1px solid var(--border)',
  borderRadius: 6, color: 'var(--text-primary)',
  outline: 'none', cursor: 'pointer',
}

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '8px 10px', fontSize: 13,
  background: 'var(--surface)', border: '1px solid var(--border)',
  borderRadius: 6, color: 'var(--text-primary)', outline: 'none',
}

const primaryBtn: React.CSSProperties = {
  padding: '7px 14px', fontSize: 12, fontWeight: 500,
  background: 'var(--accent)', color: 'white',
  border: 'none', borderRadius: 6, cursor: 'pointer',
}

const secondaryBtn: React.CSSProperties = {
  padding: '7px 14px', fontSize: 12, fontWeight: 500,
  background: 'var(--bg-active)', color: 'var(--text-primary)',
  border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer',
}
