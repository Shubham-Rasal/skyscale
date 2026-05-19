'use client'

import type { ReactNode } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Sidebar } from '@/components/Sidebar'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { api } from '@/lib/api'
import type { FaasContainer, FaasTemplate } from '@/lib/faas/types'

export default function FaasPage() {
  const { connected } = useRealtimeStream()
  const [templates, setTemplates] = useState<FaasTemplate[]>([])
  const [containers, setContainers] = useState<FaasContainer[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<string | null>(null)
  const testBodyRef = useRef<HTMLTextAreaElement | null>(null)

  const selected = useMemo(
    () => containers.find((c) => c.id === selectedId) ?? null,
    [containers, selectedId],
  )

  const refresh = useCallback(async () => {
    try {
      const data = await api.listFaasContainers()
      setContainers(data.containers)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const [t, c] = await Promise.all([
          api.listFaasTemplates(),
          api.listFaasContainers(),
        ])
        if (cancelled) return
        setTemplates(t.templates)
        setContainers(c.containers)
        setError(null)
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : String(e))
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    const needsPoll = containers.some(
      (c) =>
        c.status === 'deploying' ||
        c.status === 'stopping' ||
        c.status === 'running',
    )
    if (!needsPoll) return
    const t = setInterval(() => {
      void refresh()
    }, 2500)
    return () => clearInterval(t)
  }, [containers, refresh])

  const defaultTestBodyJson = useMemo(() => {
    if (!selected) return '{}'
    const tmpl = templates.find((x) => x.id === selected.templateId)
    return JSON.stringify(tmpl?.testBodyExample ?? {}, null, 2)
  }, [selected, templates])

  async function spinUp(templateId: string) {
    setBusy(`up:${templateId}`)
    setError(null)
    try {
      const { container } = await api.createFaasContainer(templateId)
      setSelectedId(container.id)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  async function spinDown(id: string) {
    setBusy(`down:${id}`)
    setTestResult(null)
    try {
      await api.stopFaasContainer(id)
      await refresh()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  async function runTest(id: string) {
    setBusy(`test:${id}`)
    setTestResult(null)
    try {
      const raw = testBodyRef.current?.value ?? '{}'
      let body: Record<string, unknown>
      try {
        body = JSON.parse(raw) as Record<string, unknown>
      } catch {
        throw new Error('Test body must be valid JSON')
      }
      const res = await api.testFaasContainer(id, body)
      setTestResult(JSON.stringify(res, null, 2))
      await refresh()
    } catch (e) {
      setTestResult(JSON.stringify({ error: e instanceof Error ? e.message : String(e) }, null, 2))
    } finally {
      setBusy(null)
    }
  }

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={connected} />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        <header style={{
          height: 56,
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          padding: '0 24px',
          gap: 12,
          flexShrink: 0,
        }}>
          <div style={{ flex: 1 }}>
            <h1 style={{
              fontFamily: 'var(--font-grotesk), sans-serif',
              fontSize: 15,
              fontWeight: 600,
              color: 'var(--text-primary)',
              letterSpacing: '-0.02em',
            }}>
              FaaS Containers
            </h1>
            <p style={{ fontSize: 11, color: 'var(--text-secondary)', marginTop: 2 }}>
              Deploy and stop Railway services via the public GraphQL API (server-side token).
            </p>
          </div>
          <span style={{
            fontSize: 10,
            fontWeight: 600,
            padding: '2px 8px',
            borderRadius: 4,
            background: 'var(--accent-dim)',
            color: 'var(--accent)',
            border: '1px solid var(--accent-border)',
            letterSpacing: '0.06em',
          }}>
            RAILWAY API
          </span>
        </header>

        <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
          {/* Templates */}
          <section style={{
            width: '38%',
            minWidth: 280,
            borderRight: '1px solid var(--border)',
            overflowY: 'auto',
            padding: '16px 16px 24px',
          }}>
            <SectionTitle>Templates</SectionTitle>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              {templates.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  onClick={() => spinUp(t.id)}
                  disabled={busy !== null}
                  style={{
                    textAlign: 'left',
                    padding: '12px 14px',
                    borderRadius: 10,
                    border: '1px solid var(--border)',
                    background: 'var(--surface)',
                    cursor: busy ? 'wait' : 'pointer',
                    fontFamily: 'inherit',
                    color: 'var(--text-primary)',
                    transition: 'border-color 0.15s, background 0.15s',
                  }}
                >
                  <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 4 }}>{t.name}</div>
                  <div style={{ fontSize: 11, color: 'var(--text-secondary)', lineHeight: 1.45, marginBottom: 8 }}>
                    {t.description}
                  </div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                    {t.tags.map((tag) => (
                      <span key={tag} style={{
                        fontSize: 9,
                        fontWeight: 600,
                        padding: '2px 6px',
                        borderRadius: 4,
                        background: 'var(--bg-active)',
                        color: 'var(--text-secondary)',
                        border: '1px solid var(--border)',
                      }}>
                        {tag}
                      </span>
                    ))}
                  </div>
                  <div style={{ marginTop: 10, fontSize: 10, color: 'var(--accent)' }}>
                    {busy === `up:${t.id}` ? 'Spinning up…' : 'Spin Up →'}
                  </div>
                </button>
              ))}
            </div>
          </section>

          {/* Detail */}
          <section style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
            {error && (
              <div style={{
                margin: '12px 16px 0',
                padding: '10px 12px',
                borderRadius: 8,
                background: 'var(--error-dim)',
                border: '1px solid rgba(239,68,68,0.35)',
                color: 'var(--error)',
                fontSize: 12,
              }}>
                {error}
              </div>
            )}

            <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--border)' }}>
              <SectionTitle>Active & recent</SectionTitle>
              {containers.length === 0 ? (
                <div style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                  No containers yet — pick a template to spin one up.
                </div>
              ) : (
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                  {containers.map((c) => (
                    <button
                      key={c.id}
                      type="button"
                      onClick={() => setSelectedId(c.id)}
                      style={{
                        padding: '6px 10px',
                        borderRadius: 8,
                        border: selectedId === c.id ? '1px solid var(--accent-border)' : '1px solid var(--border)',
                        background: selectedId === c.id ? 'var(--accent-dim)' : 'var(--surface)',
                        color: 'var(--text-primary)',
                        fontSize: 11,
                        cursor: 'pointer',
                        fontFamily: 'inherit',
                        display: 'flex',
                        alignItems: 'center',
                        gap: 8,
                      }}
                    >
                      <StatusDot status={c.status} />
                      <span style={{ fontWeight: 500 }}>{c.templateName}</span>
                      <span style={{ color: 'var(--text-muted)', fontFamily: 'monospace', fontSize: 10 }}>
                        {c.id.slice(0, 8)}…
                      </span>
                      <span style={{
                        fontSize: 9,
                        textTransform: 'uppercase',
                        color: 'var(--text-secondary)',
                      }}>
                        {c.status}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>

            {selected ? (
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                <div style={{
                  padding: '14px 20px',
                  display: 'flex',
                  flexWrap: 'wrap',
                  gap: 10,
                  alignItems: 'center',
                  borderBottom: '1px solid var(--border)',
                }}>
                  <div style={{ flex: 1, minWidth: 200 }}>
                    <div style={{ fontSize: 11, color: 'var(--text-secondary)', marginBottom: 4 }}>Endpoint</div>
                    <code style={{
                      display: 'block',
                      fontSize: 11,
                      padding: '8px 10px',
                      background: 'var(--surface)',
                      borderRadius: 6,
                      border: '1px solid var(--border)',
                      wordBreak: 'break-all',
                      color: selected.endpointUrl ? 'var(--success)' : 'var(--text-muted)',
                    }}>
                      {selected.endpointUrl || '— not ready —'}
                    </code>
                  </div>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <ActionBtn
                      label="Spin Down"
                      variant="danger"
                      disabled={busy !== null || selected.status === 'stopped'}
                      onClick={() => spinDown(selected.id)}
                    />
                    <ActionBtn
                      label="Run test"
                      disabled={busy !== null || selected.status !== 'running'}
                      onClick={() => runTest(selected.id)}
                    />
                  </div>
                </div>

                <div style={{ flex: 1, display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 0, minHeight: 0 }}>
                  <div style={{ borderRight: '1px solid var(--border)', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                    <PanelHeader>Logs</PanelHeader>
                    <div style={{
                      flex: 1,
                      overflowY: 'auto',
                      padding: '12px 14px',
                      fontFamily: 'ui-monospace, monospace',
                      fontSize: 11,
                      lineHeight: 1.5,
                    }}>
                      {selected.logs.length === 0 ? (
                        <span style={{ color: 'var(--text-muted)' }}>No logs yet.</span>
                      ) : (
                        selected.logs.map((line, i) => (
                          <div key={`${line.ts}-${i}`} style={{ marginBottom: 6 }}>
                            <span style={{ color: 'var(--text-muted)' }}>{line.ts.slice(11, 23)}</span>{' '}
                            <span style={{
                              color: line.level === 'error' ? 'var(--error)' : line.level === 'warn' ? 'var(--warning)' : 'var(--text-secondary)',
                            }}>
                              [{line.level}]
                            </span>{' '}
                            {line.message}
                          </div>
                        ))
                      )}
                    </div>
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                    <PanelHeader>Test request body (JSON)</PanelHeader>
                    <textarea
                      key={`tb-${selected.id}-${templates.length}`}
                      ref={testBodyRef}
                      defaultValue={defaultTestBodyJson}
                      style={{
                        flex: 1,
                        minHeight: 140,
                        margin: '0 14px 10px',
                        padding: 12,
                        borderRadius: 8,
                        border: '1px solid var(--border)',
                        background: 'var(--surface)',
                        color: 'var(--text-primary)',
                        fontFamily: 'ui-monospace, monospace',
                        fontSize: 11,
                        resize: 'vertical',
                      }}
                    />
                    <PanelHeader>Test response</PanelHeader>
                    <pre style={{
                      flex: 1,
                      overflow: 'auto',
                      margin: '0 14px 14px',
                      padding: 12,
                      borderRadius: 8,
                      border: '1px solid var(--border)',
                      background: 'var(--bg-panel)',
                      fontSize: 11,
                      color: 'var(--text-secondary)',
                    }}>
                      {testResult || 'Run a test when status is running.'}
                    </pre>
                  </div>
                </div>

                {selected.errorMessage && (
                  <div style={{ padding: '0 20px 12px', fontSize: 11, color: 'var(--error)' }}>
                    {selected.errorMessage}
                  </div>
                )}
              </div>
            ) : (
              <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)', fontSize: 13 }}>
                Select a container above or spin up a new one.
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  )
}

function SectionTitle({ children }: { children: ReactNode }) {
  return (
    <div style={{
      fontSize: 10,
      fontWeight: 600,
      color: 'var(--text-secondary)',
      letterSpacing: '0.08em',
      textTransform: 'uppercase',
      marginBottom: 12,
    }}>
      {children}
    </div>
  )
}

function PanelHeader({ children }: { children: ReactNode }) {
  return (
    <div style={{
      padding: '10px 14px',
      fontSize: 10,
      fontWeight: 600,
      color: 'var(--text-muted)',
      letterSpacing: '0.06em',
      textTransform: 'uppercase',
      borderBottom: '1px solid var(--border)',
    }}>
      {children}
    </div>
  )
}

function StatusDot({ status }: { status: FaasContainer['status'] }) {
  const color =
    status === 'running' ? 'var(--success)' :
    status === 'failed' ? 'var(--error)' :
    status === 'stopped' ? 'var(--text-muted)' :
    'var(--running)'
  return (
    <span style={{
      width: 8,
      height: 8,
      borderRadius: '50%',
      background: color,
      flexShrink: 0,
      animation: (status === 'creating' || status === 'deploying' || status === 'stopping') ? 'pulse 1.2s ease-in-out infinite' : undefined,
    }} />
  )
}

function ActionBtn({
  label,
  onClick,
  disabled,
  variant,
}: {
  label: string
  onClick: () => void
  disabled?: boolean
  variant?: 'danger'
}) {
  const danger = variant === 'danger'
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      style={{
        padding: '8px 14px',
        borderRadius: 8,
        border: `1px solid ${danger ? 'rgba(239,68,68,0.4)' : 'var(--border)'}`,
        background: danger ? 'var(--error-dim)' : 'var(--accent)',
        color: danger ? 'var(--error)' : 'white',
        fontSize: 12,
        fontWeight: 500,
        cursor: disabled ? 'not-allowed' : 'pointer',
        fontFamily: 'inherit',
        opacity: disabled ? 0.5 : 1,
      }}
    >
      {label}
    </button>
  )
}
