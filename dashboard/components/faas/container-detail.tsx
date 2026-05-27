'use client'

import { useMemo, useRef, useState } from 'react'
import {
  Check,
  Copy,
  ExternalLink,
  Loader2,
  Play,
  Square,
  Terminal,
} from 'lucide-react'
import type { FaasContainer, FaasTemplate } from '@/lib/faas/types'
import { StatusBadge } from '@/components/dashboard/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

interface ContainerDetailProps {
  container: FaasContainer
  template?: FaasTemplate
  busy: string | null
  testResult: string | null
  defaultTestBodyJson: string
  onStop: () => void
  onRunTest: (body: Record<string, unknown>) => void
}

export function ContainerDetail({
  container,
  template,
  busy,
  testResult,
  defaultTestBodyJson,
  onStop,
  onRunTest,
}: ContainerDetailProps) {
  const testBodyRef = useRef<HTMLTextAreaElement | null>(null)
  const [copied, setCopied] = useState(false)
  const stopping = busy === `down:${container.id}`
  const testing = busy === `test:${container.id}`
  const canTest = container.status === 'running' && busy === null
  const canStop = container.status !== 'stopped' && busy === null

  const metaRows = useMemo(
    () => [
      { label: 'Sandbox ID', value: container.id, mono: true },
      { label: 'Template', value: container.templateName },
      { label: 'Created', value: formatTimestamp(container.createdAt) },
      { label: 'Updated', value: formatTimestamp(container.updatedAt) },
      ...(template
        ? [
            { label: 'Health check', value: template.healthPath, mono: true },
            { label: 'Test route', value: `${template.testMethod} ${template.testPath}`, mono: true },
          ]
        : []),
      ...(container.railway?.serviceId
        ? [{ label: 'Railway service', value: container.railway.serviceId, mono: true }]
        : []),
    ],
    [container, template],
  )

  async function copyEndpoint() {
    if (!container.endpointUrl) return
    await navigator.clipboard.writeText(container.endpointUrl)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 2000)
  }

  function handleRunTest() {
    const raw = testBodyRef.current?.value ?? '{}'
    try {
      const body = JSON.parse(raw) as Record<string, unknown>
      onRunTest(body)
    } catch {
      onRunTest({ __parse_error: true })
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="shrink-0 space-y-4 border-b border-border px-6 py-5">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0 space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-lg font-semibold tracking-tight">{container.templateName}</h2>
              <StatusBadge status={container.status} />
            </div>
            <p className="font-mono text-xs text-muted-foreground">{container.id}</p>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={!canStop}
              onClick={onStop}
            >
              {stopping ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Square className="size-3.5" />
              )}
              Stop
            </Button>
            <Button size="sm" disabled={!canTest} onClick={handleRunTest}>
              {testing ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Play className="size-3.5" />
              )}
              Run test
            </Button>
          </div>
        </div>

        <Card className="bg-muted/20">
          <CardContent className="flex flex-wrap items-center gap-3 p-4">
            <div className="min-w-0 flex-1">
              <p className="mb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                Endpoint
              </p>
              <code
                className={cn(
                  'block break-all font-mono text-sm',
                  container.endpointUrl ? 'text-[var(--success)]' : 'text-muted-foreground',
                )}
              >
                {container.endpointUrl || 'Waiting for deployment…'}
              </code>
            </div>
            {container.endpointUrl && (
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" onClick={copyEndpoint}>
                  {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
                  {copied ? 'Copied' : 'Copy'}
                </Button>
                <Button variant="outline" size="sm" asChild>
                  <a href={container.endpointUrl} target="_blank" rel="noreferrer">
                    <ExternalLink className="size-3.5" />
                    Open
                  </a>
                </Button>
              </div>
            )}
          </CardContent>
        </Card>

        {container.errorMessage && (
          <div className="rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {container.errorMessage}
          </div>
        )}
      </div>

      <Tabs defaultValue="overview" className="flex min-h-0 flex-1 flex-col px-6 py-4">
        <TabsList className="h-9 w-fit">
          <TabsTrigger value="overview" className="text-xs">Overview</TabsTrigger>
          <TabsTrigger value="logs" className="text-xs">
            Logs
            {container.logs.length > 0 && (
              <span className="ml-1.5 rounded-full bg-muted px-1.5 py-0.5 text-[10px] tabular-nums">
                {container.logs.length}
              </span>
            )}
          </TabsTrigger>
          <TabsTrigger value="test" className="text-xs">Test</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4 flex-1 overflow-y-auto">
          <dl className="grid gap-3 sm:grid-cols-2">
            {metaRows.map((row) => (
              <div key={row.label} className="rounded-lg border border-border bg-card p-3">
                <dt className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  {row.label}
                </dt>
                <dd
                  className={cn(
                    'mt-1 text-sm text-foreground',
                    row.mono && 'break-all font-mono text-xs',
                  )}
                >
                  {row.value}
                </dd>
              </div>
            ))}
          </dl>
        </TabsContent>

        <TabsContent value="logs" className="mt-4 min-h-0 flex-1">
          <div className="flex h-[min(420px,calc(100vh-22rem))] flex-col overflow-hidden rounded-lg border border-border bg-[#0d0d0d]">
            <div className="flex items-center gap-2 border-b border-border/80 px-3 py-2 text-xs text-muted-foreground">
              <Terminal className="size-3.5" />
              Deployment logs
            </div>
            <ScrollArea className="flex-1 p-3">
              {container.logs.length === 0 ? (
                <p className="text-xs text-muted-foreground">No logs yet. They will appear as the sandbox deploys.</p>
              ) : (
                <div className="space-y-1 font-mono text-[11px] leading-relaxed">
                  {container.logs.map((line, i) => (
                    <div key={`${line.ts}-${i}`} className="flex gap-2">
                      <span className="shrink-0 text-muted-foreground">{line.ts.slice(11, 23)}</span>
                      <span
                        className={cn(
                          'shrink-0 uppercase',
                          line.level === 'error' && 'text-destructive',
                          line.level === 'warn' && 'text-[var(--warning)]',
                          line.level === 'info' && 'text-primary/80',
                        )}
                      >
                        {line.level}
                      </span>
                      <span className="text-foreground/90">{line.message}</span>
                    </div>
                  ))}
                </div>
              )}
            </ScrollArea>
          </div>
        </TabsContent>

        <TabsContent value="test" className="mt-4 min-h-0 flex-1">
          <div className="grid h-[min(420px,calc(100vh-22rem))] gap-4 lg:grid-cols-2">
            <div className="flex min-h-0 flex-col gap-2">
              <div className="flex items-center justify-between">
                <p className="text-xs font-medium text-muted-foreground">Request body (JSON)</p>
                {template && (
                  <span className="font-mono text-[10px] text-muted-foreground">
                    {template.testMethod} {template.testPath}
                  </span>
                )}
              </div>
              <Textarea
                key={`tb-${container.id}`}
                ref={testBodyRef}
                defaultValue={defaultTestBodyJson}
                className="min-h-0 flex-1 resize-none font-mono text-xs"
              />
            </div>

            <div className="flex min-h-0 flex-col gap-2">
              <p className="text-xs font-medium text-muted-foreground">Response</p>
              <pre className="min-h-0 flex-1 overflow-auto rounded-lg border border-border bg-muted/20 p-3 font-mono text-xs text-foreground/90">
                {testResult ?? (canTest ? 'Click “Run test” to invoke the endpoint.' : 'Available when the sandbox is running.')}
              </pre>
            </div>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function formatTimestamp(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}
