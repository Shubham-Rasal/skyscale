'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Box, Plus, Search, Server } from 'lucide-react'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { StatBadge } from '@/components/dashboard/stat-badge'
import { EmptyState } from '@/components/dashboard/empty-state'
import { StatusBadge } from '@/components/dashboard/status-badge'
import { DeployTemplateDialog } from '@/components/faas/deploy-template-dialog'
import { ContainerDetail } from '@/components/faas/container-detail'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { api } from '@/lib/api'
import type { FaasContainer, FaasTemplate } from '@/lib/faas/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'

export default function FaasPage() {
  const { connected } = useRealtimeStream()
  const [templates, setTemplates] = useState<FaasTemplate[]>([])
  const [containers, setContainers] = useState<FaasContainer[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<string | null>(null)
  const [deployOpen, setDeployOpen] = useState(false)
  const [query, setQuery] = useState('')

  const selected = useMemo(
    () => containers.find((c) => c.id === selectedId) ?? null,
    [containers, selectedId],
  )

  const selectedTemplate = useMemo(
    () => templates.find((t) => t.id === selected?.templateId),
    [templates, selected?.templateId],
  )

  const sortedContainers = useMemo(
    () => [...containers].sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()),
    [containers],
  )

  const filteredContainers = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return sortedContainers
    return sortedContainers.filter(
      (c) =>
        c.templateName.toLowerCase().includes(q) ||
        c.id.toLowerCase().includes(q) ||
        c.status.toLowerCase().includes(q),
    )
  }, [sortedContainers, query])

  const stats = useMemo(() => ({
    running: containers.filter((c) => c.status === 'running').length,
    deploying: containers.filter((c) => ['creating', 'deploying', 'stopping'].includes(c.status)).length,
    total: containers.length,
  }), [containers])

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
    if (selectedId && containers.some((c) => c.id === selectedId)) return
    if (sortedContainers.length > 0) {
      setSelectedId(sortedContainers[0].id)
    } else {
      setSelectedId(null)
    }
  }, [containers, selectedId, sortedContainers])

  useEffect(() => {
    const needsPoll = containers.some(
      (c) => c.status === 'deploying' || c.status === 'stopping' || c.status === 'running' || c.status === 'creating',
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
      setDeployOpen(false)
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

  async function runTest(id: string, body: Record<string, unknown>) {
    if ('__parse_error' in body) {
      setTestResult(JSON.stringify({ error: 'Test body must be valid JSON' }, null, 2))
      return
    }

    setBusy(`test:${id}`)
    setTestResult(null)
    try {
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
        description="Deploy isolated Railway services, monitor logs, and invoke endpoints."
        badge="Railway"
        stats={
          <>
            <StatBadge label="Running" value={stats.running} tone="success" />
            <StatBadge label="Deploying" value={stats.deploying} tone="running" />
            <StatBadge label="Total" value={stats.total} tone="muted" />
          </>
        }
        actions={
          <Button size="sm" onClick={() => setDeployOpen(true)} disabled={busy?.startsWith('up:')}>
            <Plus className="size-3.5" />
            Deploy sandbox
          </Button>
        }
      />

      {error && (
        <div className="mx-6 mt-4 rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      {containers.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center px-6 py-10">
          <EmptyState
            icon={<Box className="size-5" />}
            title="No sandboxes deployed"
            description="Deploy a pre-built template to get an isolated service with a live endpoint, logs, and a built-in test harness."
            action={{ label: 'Deploy sandbox', onClick: () => setDeployOpen(true) }}
          />
          <div className="mt-8 grid w-full max-w-4xl gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {templates.slice(0, 3).map((template) => (
              <button
                key={template.id}
                type="button"
                onClick={() => setDeployOpen(true)}
                className="rounded-lg border border-border bg-card p-4 text-left transition-colors hover:border-primary/40 hover:bg-accent/20"
              >
                <p className="text-sm font-medium">{template.name}</p>
                <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
                  {template.description}
                </p>
              </button>
            ))}
          </div>
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 overflow-hidden">
          <aside className="flex w-72 shrink-0 flex-col border-r border-border bg-muted/10">
            <div className="space-y-3 border-b border-border p-4">
              <div className="flex items-center justify-between">
                <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Sandboxes
                </p>
                <span className="text-xs tabular-nums text-muted-foreground">{filteredContainers.length}</span>
              </div>
              <div className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search sandboxes…"
                  className="h-8 bg-background pl-8 text-xs"
                />
              </div>
            </div>

            <ScrollArea className="flex-1">
              <div className="space-y-1 p-2">
                {filteredContainers.length === 0 ? (
                  <p className="px-2 py-6 text-center text-xs text-muted-foreground">No matches.</p>
                ) : (
                  filteredContainers.map((container) => (
                    <button
                      key={container.id}
                      type="button"
                      onClick={() => {
                        setSelectedId(container.id)
                        setTestResult(null)
                      }}
                      className={cn(
                        'w-full rounded-lg border px-3 py-2.5 text-left transition-colors',
                        selectedId === container.id
                          ? 'border-primary/40 bg-primary/10'
                          : 'border-transparent hover:border-border hover:bg-accent/30',
                      )}
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium">{container.templateName}</p>
                          <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
                            {container.id}
                          </p>
                        </div>
                        <StatusBadge status={container.status} className="shrink-0 scale-90" />
                      </div>
                      <p className="mt-2 text-[10px] text-muted-foreground">
                        Updated {relativeTime(container.updatedAt)}
                      </p>
                    </button>
                  ))
                )}
              </div>
            </ScrollArea>
          </aside>

          <section className="flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
            {selected ? (
              <ContainerDetail
                key={selected.id}
                container={selected}
                template={selectedTemplate}
                busy={busy}
                testResult={testResult}
                defaultTestBodyJson={defaultTestBodyJson}
                onStop={() => spinDown(selected.id)}
                onRunTest={(body) => runTest(selected.id, body)}
              />
            ) : (
              <div className="flex flex-1 items-center justify-center">
                <EmptyState
                  icon={<Server className="size-5" />}
                  title="Select a sandbox"
                  description="Choose a deployment from the list to view its endpoint, logs, and test console."
                />
              </div>
            )}
          </section>
        </div>
      )}

      <DeployTemplateDialog
        open={deployOpen}
        onOpenChange={setDeployOpen}
        templates={templates}
        busyTemplateId={busy?.startsWith('up:') ? busy.slice(3) : null}
        onDeploy={spinUp}
      />
    </DashboardShell>
  )
}

function relativeTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const diff = Date.now() - date.getTime()
  if (diff < 60_000) return 'just now'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}
