'use client'

import { useEffect, useState } from 'react'
import { ExternalLink } from 'lucide-react'
import { Execution } from '@/types'
import { api } from '@/lib/api'
import { ContentPanel } from '@/components/dashboard/content-panel'
import { StatusBadge } from '@/components/dashboard/status-badge'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

interface Props {
  executionId: string | null
  onClose: () => void
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
  return d.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
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
      setLoading(prev => (prev === false ? false : true))
      api.getExecution(executionId)
        .then(d => {
          if (!cancelled) {
            setExec(d)
            setLoading(false)
          }
        })
        .catch(() => {
          if (!cancelled) {
            setExec(null)
            setLoading(false)
          }
        })
    }
    poll()
    const interval = setInterval(() => {
      if (cancelled) return
      api
        .getExecution(executionId)
        .then(d => {
          if (!cancelled) setExec(d)
          if (d.Status === 'completed' || d.Status === 'failed' || d.Status === 'error') {
            clearInterval(interval)
            api
              .listArtifacts(executionId)
              .then(a => {
                if (!cancelled) setArtifacts(a)
              })
              .catch(() => {})
          }
        })
        .catch(() => {})
    }, 4000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [executionId])

  return (
    <Sheet open={!!executionId} onOpenChange={open => !open && onClose()}>
      <SheetContent className="flex w-full flex-col overflow-hidden bg-background sm:max-w-md">
        <SheetHeader className="shrink-0 pr-8">
          <SheetTitle>Run Detail</SheetTitle>
          {exec && (
            <SheetDescription asChild>
              <div className="flex flex-wrap items-center gap-2 pt-1">
                <StatusBadge status={exec.Status} />
                <span className="font-mono text-[11px] text-muted-foreground">{exec.ID}</span>
              </div>
            </SheetDescription>
          )}
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto py-4">
          {loading && <p className="text-sm text-muted-foreground">Loading…</p>}

          {!loading && !exec && executionId && (
            <p className="text-sm text-muted-foreground">Execution not found.</p>
          )}

          {!loading && exec && (
            <div className="space-y-4">
              <ContentPanel title="Execution" contentClassName="space-y-2.5 p-4">
                <DetailRow label="Function" value={exec.FunctionName || exec.FunctionID} mono />
                <DetailRow label="Job Type" value={exec.JobType || 'faas_function'} />
                <DetailRow
                  label="Hardware"
                  value={exec.HardwareType === 'gpu' ? `GPU · ${exec.GPUModel}` : 'CPU (Firecracker)'}
                />
                <DetailRow label="Duration" value={fmtDuration(exec.Duration)} />
                <DetailRow label="Started" value={fmtTime(exec.StartTime)} />
                <DetailRow label="Ended" value={fmtTime(exec.EndTime)} />
              </ContentPanel>

              {exec.HardwareType === 'gpu' && exec.VMID && (
                <ContentPanel title="Akash Deployment" contentClassName="space-y-2.5 p-4">
                  <DetailRow label="Deployment (dseq)" value={exec.VMID} mono />
                  <DetailRow
                    label="Status"
                    value={exec.Status === 'error' || exec.Status === 'completed' ? 'Closed' : 'Active'}
                  />
                  {/^\d+$/.test(exec.VMID) && (
                    <a
                      href={`https://console.akash.network/deployments/${exec.VMID}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                    >
                      View on Akash Console
                      <ExternalLink className="size-3" />
                    </a>
                  )}
                </ContentPanel>
              )}

              {exec.Error && (
                <ContentPanel title="Error" contentClassName="p-4">
                  <pre className="whitespace-pre-wrap break-words rounded-md border border-destructive/20 bg-destructive/10 p-3 font-mono text-[11px] leading-relaxed text-destructive">
                    {exec.Error}
                  </pre>
                </ContentPanel>
              )}

              <ContentPanel title="Logs" contentClassName="p-4">
                {exec.Logs ? (
                  <pre className="max-h-[300px] overflow-y-auto whitespace-pre-wrap break-words rounded-md border border-border bg-muted p-3 font-mono text-[11px] leading-relaxed text-foreground">
                    {exec.Logs}
                  </pre>
                ) : (
                  <span className="text-xs text-muted-foreground">No logs captured.</span>
                )}
              </ContentPanel>

              <ContentPanel title="Artifacts" contentClassName="p-4">
                {artifacts.length === 0 ? (
                  <span className="text-xs text-muted-foreground">No artifacts.</span>
                ) : (
                  <div className="flex flex-col gap-2">
                    {artifacts.map(name => (
                      <a
                        key={name}
                        href={api.artifactDownloadURL(executionId!, name)}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-xs text-primary hover:underline"
                      >
                        {name}
                      </a>
                    ))}
                  </div>
                )}
              </ContentPanel>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function DetailRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="w-28 shrink-0 text-xs text-muted-foreground">{label}</span>
      <span className={mono ? 'break-all font-mono text-xs text-foreground' : 'text-sm text-foreground'}>
        {value || '—'}
      </span>
    </div>
  )
}
