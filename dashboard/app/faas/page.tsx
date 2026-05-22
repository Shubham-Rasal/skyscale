'use client'

import type { ReactNode } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { StatusBadge } from '@/components/dashboard/status-badge'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { api } from '@/lib/api'
import type { FaasContainer, FaasTemplate } from '@/lib/faas/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

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
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    const needsPoll = containers.some(
      (c) => c.status === 'deploying' || c.status === 'stopping' || c.status === 'running',
    )
    if (!needsPoll) return
    const t = setInterval(() => { void refresh() }, 2500)
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
    <DashboardShell connected={connected}>
      <PageHeader
        title="Sandboxes"
        description="Deploy and stop Railway services via the public GraphQL API."
        badge="Railway API"
      />

      <div className="flex min-h-0 flex-1 overflow-hidden">
        <aside className="w-[min(380px,38%)] shrink-0 overflow-y-auto border-r border-border p-4">
          <SectionLabel>Templates</SectionLabel>
          <div className="space-y-2">
            {templates.map((t) => (
              <Card
                key={t.id}
                className="cursor-pointer transition-colors hover:border-primary/40 hover:bg-accent/20"
              >
                <CardHeader className="p-4 pb-2">
                  <CardTitle className="text-sm">{t.name}</CardTitle>
                  <CardDescription className="text-xs leading-relaxed">{t.description}</CardDescription>
                </CardHeader>
                <CardContent className="p-4 pt-0">
                  <div className="mb-3 flex flex-wrap gap-1">
                    {t.tags.map((tag) => (
                      <Badge key={tag} variant="secondary" className="text-[10px]">{tag}</Badge>
                    ))}
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-0 text-primary hover:bg-transparent"
                    disabled={busy !== null}
                    onClick={() => spinUp(t.id)}
                  >
                    {busy === `up:${t.id}` ? 'Spinning up…' : 'Spin Up →'}
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>
        </aside>

        <section className="flex min-w-0 flex-1 flex-col overflow-hidden">
          {error && (
            <div className="mx-4 mt-3 rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="border-b border-border p-4">
            <SectionLabel>Active & recent</SectionLabel>
            {containers.length === 0 ? (
              <p className="text-xs text-muted-foreground">No sandboxes yet — pick a template to spin one up.</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {containers.map((c) => (
                  <Button
                    key={c.id}
                    variant="outline"
                    size="sm"
                    className={cn(
                      'h-auto gap-2 py-1.5',
                      selectedId === c.id && 'border-primary/40 bg-primary/10',
                    )}
                    onClick={() => setSelectedId(c.id)}
                  >
                    <StatusDot status={c.status} />
                    <span className="font-medium">{c.templateName}</span>
                    <span className="font-mono text-[10px] text-muted-foreground">{c.id.slice(0, 8)}…</span>
                    <StatusBadge status={c.status} className="scale-90" />
                  </Button>
                ))}
              </div>
            )}
          </div>

          {selected ? (
            <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
              <div className="flex flex-wrap items-center gap-3 border-b border-border p-4">
                <div className="min-w-[200px] flex-1">
                  <p className="mb-1 text-xs text-muted-foreground">Endpoint</p>
                  <code className={cn(
                    'block break-all rounded-md border border-border bg-muted/40 px-2.5 py-2 font-mono text-[11px]',
                    selected.endpointUrl ? 'text-[var(--success)]' : 'text-muted-foreground',
                  )}>
                    {selected.endpointUrl || '— not ready —'}
                  </code>
                </div>
                <Button
                  variant="destructive"
                  size="sm"
                  disabled={busy !== null || selected.status === 'stopped'}
                  onClick={() => spinDown(selected.id)}
                >
                  Spin Down
                </Button>
                <Button
                  size="sm"
                  disabled={busy !== null || selected.status !== 'running'}
                  onClick={() => runTest(selected.id)}
                >
                  Run test
                </Button>
              </div>

              <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-2">
                <div className="flex min-h-0 flex-col border-b border-border lg:border-b-0 lg:border-r">
                  <PanelHeader>Logs</PanelHeader>
                  <ScrollArea className="flex-1 p-3 font-mono text-[11px] leading-relaxed">
                    {selected.logs.length === 0 ? (
                      <span className="text-muted-foreground">No logs yet.</span>
                    ) : (
                      selected.logs.map((line, i) => (
                        <div key={`${line.ts}-${i}`} className="mb-1.5">
                          <span className="text-muted-foreground">{line.ts.slice(11, 23)}</span>{' '}
                          <span className={cn(
                            line.level === 'error' && 'text-destructive',
                            line.level === 'warn' && 'text-[var(--warning)]',
                            line.level !== 'error' && line.level !== 'warn' && 'text-muted-foreground',
                          )}>
                            [{line.level}]
                          </span>{' '}
                          {line.message}
                        </div>
                      ))
                    )}
                  </ScrollArea>
                </div>

                <div className="flex min-h-0 flex-col">
                  <PanelHeader>Test request body (JSON)</PanelHeader>
                  <Textarea
                    key={`tb-${selected.id}-${templates.length}`}
                    ref={testBodyRef}
                    defaultValue={defaultTestBodyJson}
                    className="mx-3 mb-2 min-h-[140px] flex-1 resize-y font-mono text-[11px]"
                  />
                  <PanelHeader>Test response</PanelHeader>
                  <pre className="mx-3 mb-3 flex-1 overflow-auto rounded-md border border-border bg-muted/30 p-3 font-mono text-[11px] text-muted-foreground">
                    {testResult || 'Run a test when status is running.'}
                  </pre>
                </div>
              </div>

              {selected.errorMessage && (
                <p className="px-4 pb-3 text-xs text-destructive">{selected.errorMessage}</p>
              )}
            </div>
          ) : (
            <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
              Select a sandbox above or spin up a new one.
            </div>
          )}
        </section>
      </div>
    </DashboardShell>
  )
}

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
      {children}
    </p>
  )
}

function PanelHeader({ children }: { children: ReactNode }) {
  return (
    <div className="border-b border-border px-3 py-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
      {children}
    </div>
  )
}

function StatusDot({ status }: { status: FaasContainer['status'] }) {
  const color =
    status === 'running' ? 'bg-[var(--success)]' :
    status === 'failed' ? 'bg-destructive' :
    status === 'stopped' ? 'bg-muted-foreground/40' :
    'bg-[var(--running)]'

  return (
    <span
      className={cn(
        'size-2 shrink-0 rounded-full',
        color,
        (status === 'creating' || status === 'deploying' || status === 'stopping') &&
          'animate-[pulse_1.2s_ease-in-out_infinite]',
      )}
    />
  )
}
