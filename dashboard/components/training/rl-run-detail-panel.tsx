'use client'

import { useCallback, useEffect, useState } from 'react'
import { ExternalLink, Square } from 'lucide-react'
import { api, RLRunDetail, RLEvent } from '@/lib/api'
import { StatusBadge } from '@/components/dashboard/status-badge'
import { MiniMetricChart } from '@/components/training/rl-charts'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

interface RlRunDetailPanelProps {
  runId: string
  onClose?: () => void
}

export function RlRunDetailPanel({ runId, onClose }: RlRunDetailPanelProps) {
  const [detail, setDetail] = useState<RLRunDetail | null>(null)
  const [events, setEvents] = useState<RLEvent[]>([])

  const refresh = useCallback(async () => {
    try {
      const [d, ev] = await Promise.all([
        api.getRLRun(runId),
        api.getRLEvents(runId, 120),
      ])
      setDetail(d)
      setEvents(ev.events ?? [])
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

  const metrics = detail?.metrics ?? []
  const canStop = detail && (detail.run.Status === 'running' || detail.run.Status === 'starting')

  return (
    <aside className="flex w-[min(480px,42%)] shrink-0 flex-col border-l border-border bg-background">
      <div className="shrink-0 space-y-3 border-b border-border p-4">
        <div className="grid grid-cols-2 gap-2">
          <MiniMetricChart title="Episode reward" data={metrics} dataKey="episode_reward" color="#34d399" />
          <MiniMetricChart title="Training loss" data={metrics} dataKey="loss" color="#fbbf24" />
        </div>
        <MiniMetricChart title="GPU utilization" data={metrics} dataKey="gpu_util" color="#a78bfa" reservedLabel="Baseline" />
      </div>

      <Tabs defaultValue="logs" className="flex min-h-0 flex-1 flex-col">
        <div className="flex items-center justify-between border-b border-border px-4 pt-3">
          <TabsList className="h-8 bg-transparent p-0">
            <TabsTrigger
              value="logs"
              className="rounded-none border-b-2 border-transparent px-3 pb-2 pt-1 data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none"
            >
              Logs
            </TabsTrigger>
            <TabsTrigger
              value="details"
              className="rounded-none border-b-2 border-transparent px-3 pb-2 pt-1 data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none"
            >
              Details
            </TabsTrigger>
            <TabsTrigger
              value="events"
              className="rounded-none border-b-2 border-transparent px-3 pb-2 pt-1 data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none"
            >
              Events
            </TabsTrigger>
          </TabsList>
          {detail?.grafana_url && (
            <Button variant="ghost" size="sm" className="h-7 text-xs" asChild>
              <a href={detail.grafana_url} target="_blank" rel="noopener noreferrer">
                View all logs
                <ExternalLink className="size-3" />
              </a>
            </Button>
          )}
        </div>

        <TabsContent value="logs" className="mt-0 min-h-0 flex-1 data-[state=inactive]:hidden">
          <ScrollArea className="h-[calc(100vh-22rem)]">
            <div className="space-y-0 p-2 font-mono text-[11px] leading-relaxed">
              {!detail && <p className="p-3 text-muted-foreground">Loading…</p>}
              {detail && events.length === 0 && (
                <p className="p-3 text-muted-foreground">No log lines yet.</p>
              )}
              {[...events].reverse().map((ev, i) => (
                <LogLine key={`${ev.timestamp}-${i}`} event={ev} />
              ))}
            </div>
          </ScrollArea>
        </TabsContent>

        <TabsContent value="details" className="mt-0 flex-1 overflow-y-auto p-4 data-[state=inactive]:hidden">
          {!detail ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : (
            <dl className="space-y-3 text-sm">
              <DetailItem label="Run ID" value={detail.run.ID} mono />
              <DetailItem label="Status" value={<StatusBadge status={detail.run.Status} />} />
              <DetailItem label="Stage" value={stageLabel(detail.stage)} />
              <DetailItem label="Base model" value={detail.run.BaseModel} mono />
              <DetailItem label="GPU" value={detail.run.GPUModel.toUpperCase()} />
              <DetailItem label="Workers" value={String(detail.run.NumWorkers)} />
              <DetailItem label="Buffer" value={`${detail.buffer_size} trajectories`} />
              {detail.run.PolicyServerURL && (
                <DetailItem label="Policy URL" value={detail.run.PolicyServerURL} mono />
              )}
              <div className="pt-2">
                {canStop && (
                  <Button variant="destructive" size="sm" onClick={stop}>
                    <Square className="size-3" />
                    Stop run
                  </Button>
                )}
              </div>
            </dl>
          )}
        </TabsContent>

        <TabsContent value="events" className="mt-0 flex-1 overflow-y-auto data-[state=inactive]:hidden">
          <ScrollArea className="h-[calc(100vh-22rem)]">
            <div className="divide-y divide-border">
              {events.length === 0 ? (
                <p className="p-4 text-sm text-muted-foreground">No pipeline events yet.</p>
              ) : (
                [...events].reverse().map((ev, i) => (
                  <div key={`${ev.timestamp}-${i}`} className="px-4 py-3 text-xs">
                    <div className="flex items-center gap-2 text-muted-foreground">
                      <span className="font-mono">{formatEventTime(ev.timestamp)}</span>
                      <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase">{ev.component}</span>
                      <span className={cn(
                        'uppercase',
                        ev.level === 'error' ? 'text-destructive' : 'text-primary/80',
                      )}>
                        {ev.level}
                      </span>
                    </div>
                    <p className="mt-1 text-foreground">{ev.message}</p>
                  </div>
                ))
              )}
            </div>
          </ScrollArea>
        </TabsContent>
      </Tabs>
    </aside>
  )
}

function LogLine({ event }: { event: RLEvent }) {
  return (
    <div className="flex gap-2 px-2 py-1 hover:bg-accent/30">
      <span className="mt-1 size-2 shrink-0 rounded-sm bg-[#f97316]" />
      <div className="min-w-0 flex-1">
        <span className="text-muted-foreground">{formatLogTimestamp(event.timestamp)}</span>{' '}
        <span className={cn(
          'uppercase',
          event.level === 'error' ? 'text-destructive' : 'text-primary/90',
        )}>
          {event.level}
        </span>{' '}
        <span className="text-foreground/90">{event.message}</span>
      </div>
    </div>
  )
}

function DetailItem({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="grid grid-cols-[100px_1fr] gap-2">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={mono ? 'break-all font-mono text-xs' : 'text-sm'}>{value}</dd>
    </div>
  )
}

function stageLabel(stage?: string) {
  switch (stage) {
    case 'waiting_for_policy': return 'Waiting for policy server'
    case 'waiting_for_rollouts': return 'Waiting for rollouts'
    case 'training': return 'Training'
    case 'running': return 'Running'
    default: return stage ?? 'Starting'
  }
}

function formatEventTime(ts: string) {
  try {
    return new Date(ts).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
  } catch {
    return ts
  }
}

function formatLogTimestamp(ts: string) {
  try {
    const d = new Date(ts)
    const base = d.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).replace(',', '')
    const ms = String(d.getMilliseconds()).padStart(3, '0')
    return `${base}.${ms}`
  } catch {
    return ts
  }
}
