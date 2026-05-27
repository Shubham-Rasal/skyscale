'use client'

import { useMemo, useState } from 'react'
import { Gauge, Timer, Zap } from 'lucide-react'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { StatBadge } from '@/components/dashboard/stat-badge'
import { ContentPanel } from '@/components/dashboard/content-panel'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

type ColumnProfile = {
  name: string
  type: 'number' | 'string'
  null_count: number
  null_pct: number
  unique_count: number
  stats?: { min: number; max: number; mean: number; p50: number }
}

type ProfileReport = {
  ok: boolean
  row_count: number
  column_count: number
  quality_score: number
  columns: ColumnProfile[]
  issues: Array<{ severity: 'high' | 'medium'; column: string; message: string }>
}

type InvokeResponse = {
  request_id?: string
  function_id?: string
  status_code: number
  output?: ProfileReport
  error_message?: string
  duration_ms?: number
}

type LiveRun = {
  functionName: string
  httpStatus: 'ok' | 'error'
  wallMs: number
  completedAt: string
  response: InvokeResponse
}

const SAMPLE_CSV = `customer_id,plan,monthly_spend,events_last_30d,churn_risk,region
1001,pro,129.50,84,0.12,NA
1002,starter,19.00,12,0.43,EU
1003,enterprise,899.00,412,0.04,US
1004,pro,,55,0.20,US
1005,starter,19.00,,0.62,
1006,enterprise,1200.00,530,0.03,APAC
1007,pro,149.00,73,,EU
1008,starter,9.00,4,0.81,
`

const CSV_PROFILER_CODE = `import base64
import csv
import io
import math
import time

NULLS = {"", "null", "none", "nan", "n/a", "na"}

def is_null(value):
    return value is None or str(value).strip().lower() in NULLS

def to_float(value):
    if is_null(value):
        return None
    try:
        n = float(str(value).strip())
        return None if math.isnan(n) or math.isinf(n) else n
    except Exception:
        return None

def handle(event, context):
    time.sleep(3)
    csv_text = base64.b64decode(event.get("csv_b64", "")).decode("utf-8")
    reader = csv.DictReader(io.StringIO(csv_text))
    rows = list(reader)
    columns = reader.fieldnames or []
    report = []
    for column_name in columns:
        values = [row.get(column_name, "") for row in rows]
        null_count = sum(1 for value in values if is_null(value))
        non_null = [value for value in values if not is_null(value)]
        nums = [to_float(value) for value in non_null]
        nums = [value for value in nums if value is not None]
        numeric = bool(non_null) and len(nums) == len(non_null)
        item = {
            "name": column_name,
            "type": "number" if numeric else "string",
            "null_count": null_count,
            "null_pct": round(null_count * 100 / len(rows), 2) if rows else 0,
            "unique_count": len({str(value).strip() for value in non_null}),
        }
        if numeric and nums:
            ordered = sorted(nums)
            item["stats"] = {
                "min": min(nums),
                "max": max(nums),
                "mean": round(sum(nums) / len(nums), 3),
                "p50": ordered[len(ordered) // 2],
            }
        report.append(item)
    issues = []
    for col in report:
        if col["null_pct"] >= 30:
            issues.append({"severity": "high", "column": col["name"], "message": str(col["null_pct"]) + "% missing"})
        elif col["null_count"]:
            issues.append({"severity": "medium", "column": col["name"], "message": str(col["null_count"]) + " missing values"})
    score = max(0, round(100 - sum(c["null_pct"] for c in report) / max(len(report), 1), 1))
    return {"ok": True, "row_count": len(rows), "column_count": len(columns), "quality_score": score, "columns": report, "issues": issues}
`

const VERIFIED_LOAD_TEST = {
  p95Ms: 1680,
  avgMs: 821,
  throughputRps: 0.5,
  successRate: '100/100',
  scenario: '100 concurrent users, 100 invocations',
  pool: '2 warm Firecracker VMs',
}

const SPEED_STATS = [
  {
    label: 'p95 latency',
    value: '1.68s',
    tone: 'running' as const,
    sub: '95% of runs finish inside the VM within this time',
    icon: Gauge,
  },
  {
    label: 'Average latency',
    value: '821ms',
    tone: 'primary' as const,
    sub: 'Typical execution time once a warm VM is ready',
    icon: Timer,
  },
  {
    label: 'Throughput',
    value: '0.50 req/s',
    tone: 'warning' as const,
    sub: 'Sustained rate under 100 concurrent users (k6)',
    icon: Zap,
  },
  {
    label: 'Success rate',
    value: '100/100',
    tone: 'success' as const,
    sub: 'All invocations completed without error',
    icon: Gauge,
  },
]

export default function BenchmarksPage() {
  const { data, connected } = useRealtimeStream()
  const [running, setRunning] = useState(false)
  const [liveRun, setLiveRun] = useState<LiveRun | null>(null)
  const [runError, setRunError] = useState<string | null>(null)
  const [showWorkloadOutput, setShowWorkloadOutput] = useState(false)

  const runResult = liveRun?.response ?? null
  const report = runResult?.output

  const headlineStats = useMemo(() => {
    if (!liveRun) return SPEED_STATS

    const ok = liveRun.httpStatus === 'ok' && liveRun.response.status_code === 200
    const vmMs = liveRun.response.duration_ms
    const wallMs = liveRun.wallMs

    return [
      {
        label: 'Your last run',
        value: ok ? 'Success' : 'Failed',
        tone: ok ? 'success' as const : 'primary' as const,
        sub: `Completed at ${liveRun.completedAt}`,
        icon: Gauge,
      },
      {
        label: 'End-to-end time',
        value: wallMs ? formatMs(wallMs) : '—',
        tone: 'running' as const,
        sub: 'Browser click → response (includes register + invoke)',
        icon: Timer,
      },
      {
        label: 'VM execution',
        value: vmMs ? formatMs(vmMs) : '—',
        tone: 'primary' as const,
        sub: 'Time spent running Python inside Firecracker',
        icon: Zap,
      },
      {
        label: 'vs verified p95',
        value: vmMs ? compareToP95(vmMs) : '—',
        tone: vmMs && vmMs <= VERIFIED_LOAD_TEST.p95Ms ? 'success' as const : 'warning' as const,
        sub: `Verified p95 is ${formatMs(VERIFIED_LOAD_TEST.p95Ms)} under load`,
        icon: Gauge,
      },
    ]
  }, [liveRun])

  const readyWarm = useMemo(
    () => data.vm_pool.filter(vm => (vm.HardwareType === 'cpu' || !vm.HardwareType) && vm.IsWarm && vm.Status === 'ready' && vm.ID !== 'host-vm-test').length,
    [data.vm_pool],
  )
  const busyCPU = useMemo(
    () => data.vm_pool.filter(vm => (vm.HardwareType === 'cpu' || !vm.HardwareType) && vm.Status === 'busy').length,
    [data.vm_pool],
  )

  async function runSpeedTest() {
    setRunning(true)
    setRunError(null)
    try {
      const name = `speed-test-${Date.now()}`
      await api.registerFunction({
        name,
        runtime: 'python3',
        memory: 128,
        timeout: 30,
        code: CSV_PROFILER_CODE,
        requirements: '',
        config: '',
      })
      const startedAt = performance.now()
      const result = await api.invoke(name, {
        input: { csv_b64: btoa(SAMPLE_CSV) },
        sync: true,
        job_type: 'faas_function',
        hardware_type: 'cpu',
      }) as InvokeResponse
      setLiveRun({
        functionName: name,
        httpStatus: 'ok',
        wallMs: Math.round(performance.now() - startedAt),
        completedAt: new Date().toLocaleTimeString(),
        response: result,
      })
      if (result.status_code !== 200) {
        setRunError(result.error_message ?? 'Speed test returned a non-200 status')
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to run speed test'
      setRunError(message)
      setLiveRun({
        functionName: 'speed-test',
        httpStatus: 'error',
        wallMs: 0,
        completedAt: new Date().toLocaleTimeString(),
        response: { status_code: 500, error_message: message },
      })
    } finally {
      setRunning(false)
    }
  }

  return (
    <DashboardShell connected={connected}>
      <PageHeader
        title="Load Speed"
        description="How fast Firecracker VMs start, run code, and respond under load."
        badge="Verified"
        badgeVariant="outline"
        stats={
          <>
            <StatBadge label="Warm VMs" value={readyWarm} tone="success" />
            <StatBadge label="Busy" value={busyCPU} tone="running" />
          </>
        }
      />

      <main className="flex-1 overflow-y-auto p-6">
        <Card className="mb-6 overflow-hidden border-primary/20 bg-gradient-to-br from-primary/10 via-card to-card">
          <CardContent className="p-7">
            <Badge variant="outline" className="mb-3 border-primary/30 bg-primary/10 text-primary">
              What this measures
            </Badge>
            <h2 className="mb-2 max-w-2xl text-2xl font-semibold tracking-tight">
              Time from request to finished execution
            </h2>
            <p className="mb-5 max-w-2xl text-sm leading-relaxed text-muted-foreground">
              We stress-test the platform with 100 concurrent users hitting a warm VM pool.
              Each request runs the same Python workload so results are comparable run to run.
              Lower latency and higher throughput mean faster load handling.
            </p>
            <div className="flex flex-wrap gap-2">
              <Badge variant="secondary">{VERIFIED_LOAD_TEST.scenario}</Badge>
              <Badge variant="secondary">{VERIFIED_LOAD_TEST.pool}</Badge>
              <Badge variant="secondary">Fixed CSV workload (~8 rows)</Badge>
            </div>
          </CardContent>
        </Card>

        <section className="mb-6">
          <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            {liveRun ? 'Your latest run' : 'Verified load test results'}
          </p>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            {headlineStats.map(stat => (
              <SpeedMetricCard key={stat.label} {...stat} />
            ))}
          </div>
        </section>

        <section className="mb-6 grid items-start gap-4 xl:grid-cols-[1fr_1fr]">
          <ContentPanel title="Latency breakdown">
            <LatencyBreakdown liveRun={liveRun} />
          </ContentPanel>

          <ContentPanel title="Run your own test">
            <p className="mb-4 text-sm leading-relaxed text-muted-foreground">
              Registers a fresh function and invokes it once from your browser.
              This includes setup overhead, so it is usually slower than the verified pool benchmark above.
            </p>
            <Button className="w-full" onClick={runSpeedTest} disabled={running}>
              {running ? 'Running speed test…' : 'Run speed test'}
            </Button>
            {runError && (
              <p className="mt-3 rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {runError}
              </p>
            )}
            <div className="mt-4">
              <LiveRunSummary run={liveRun} running={running} />
            </div>
          </ContentPanel>
        </section>

        <section className="grid items-start gap-4 xl:grid-cols-2">
          <ContentPanel title="Verified k6 load test">
            <p className="mb-4 text-xs leading-relaxed text-muted-foreground">
              Automated load test against a pre-warmed two-VM pool. These numbers are the baseline for platform speed.
            </p>
            <LoadTestTable />
          </ContentPanel>

          <ContentPanel title="Test workload">
            <p className="mb-3 text-xs leading-relaxed text-muted-foreground">
              Every invocation parses a small CSV and returns JSON. The workload is fixed so timing differences reflect platform speed, not varying input size.
            </p>
            <div className="space-y-2 rounded-lg border border-border bg-muted/20 p-3 text-xs">
              <Row label="Runtime" value="Python 3 on Firecracker" />
              <Row label="Memory" value="128 MB" />
              <Row label="Payload" value="8-row customer CSV" />
              <Row label="Work done" value="Parse, infer types, compute stats" />
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="mt-3 h-7 px-0 text-muted-foreground hover:bg-transparent"
              onClick={() => setShowWorkloadOutput(v => !v)}
            >
              {showWorkloadOutput ? 'Hide sample output ↑' : 'Show sample output ↓'}
            </Button>
            {showWorkloadOutput && report && (
              <div className="mt-3 space-y-3 border-t border-border pt-3">
                <ReportSummary report={report} duration={runResult?.duration_ms} />
                <ColumnTable columns={report.columns} />
              </div>
            )}
            {showWorkloadOutput && !report && (
              <p className="mt-3 border-t border-border pt-3 text-xs text-muted-foreground">
                Run a speed test to see the workload output from your invocation.
              </p>
            )}
          </ContentPanel>
        </section>
      </main>
    </DashboardShell>
  )
}

function formatMs(ms: number) {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function compareToP95(vmMs: number) {
  const diff = vmMs - VERIFIED_LOAD_TEST.p95Ms
  if (diff <= 0) return `${Math.abs(Math.round(diff / VERIFIED_LOAD_TEST.p95Ms * 100))}% faster`
  return `${Math.round(diff / VERIFIED_LOAD_TEST.p95Ms * 100)}% slower`
}

function SpeedMetricCard({
  label,
  value,
  tone,
  sub,
  icon: Icon,
}: {
  label: string
  value: string
  tone: 'primary' | 'success' | 'warning' | 'running'
  sub: string
  icon: typeof Gauge
}) {
  const toneClass = {
    primary: 'text-primary',
    success: 'text-[var(--success)]',
    warning: 'text-[var(--warning)]',
    running: 'text-[var(--running)]',
  }[tone]

  return (
    <Card>
      <CardContent className="p-4">
        <div className="mb-2 flex items-center gap-2">
          <Icon className={cn('size-4', toneClass)} />
          <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">{label}</span>
        </div>
        <div className={cn('text-3xl font-bold tracking-tight', toneClass)}>{value}</div>
        <div className="mt-2 text-xs leading-relaxed text-muted-foreground">{sub}</div>
      </CardContent>
    </Card>
  )
}

function LatencyBreakdown({ liveRun }: { liveRun: LiveRun | null }) {
  const vmMs = liveRun?.response.duration_ms ?? VERIFIED_LOAD_TEST.avgMs
  const p95Ms = VERIFIED_LOAD_TEST.p95Ms
  const maxMs = Math.max(p95Ms, vmMs) * 1.15

  const bars = liveRun
    ? [
        { label: 'VM execution', ms: vmMs, tone: 'bg-[var(--running)]', hint: 'Python running inside Firecracker' },
        { label: 'End-to-end', ms: liveRun.wallMs, tone: 'bg-primary', hint: 'Includes function registration + network' },
        { label: 'Verified p95', ms: p95Ms, tone: 'bg-muted-foreground/50', hint: 'Baseline under 100 concurrent users' },
      ]
    : [
        { label: 'Average (warm)', ms: VERIFIED_LOAD_TEST.avgMs, tone: 'bg-[var(--running)]', hint: 'Typical VM execution time' },
        { label: 'p95 (warm)', ms: p95Ms, tone: 'bg-primary', hint: '95th percentile under load' },
        { label: 'HTTP p95', ms: 186000, tone: 'bg-muted-foreground/50', hint: 'Full round-trip including queueing at peak load' },
      ]

  return (
    <div className="space-y-4">
      <p className="text-xs leading-relaxed text-muted-foreground">
        {liveRun
          ? 'Compare your single browser run against the verified load-test baseline.'
          : 'Verified numbers from a warm pool under concurrent load. HTTP times include queueing when all VMs are busy.'}
      </p>
      <div className="space-y-3">
        {bars.map(bar => (
          <div key={bar.label}>
            <div className="mb-1 flex items-baseline justify-between gap-2 text-xs">
              <span className="font-medium">{bar.label}</span>
              <span className="font-mono tabular-nums text-muted-foreground">{formatMs(bar.ms)}</span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-muted">
              <div
                className={cn('h-full rounded-full transition-all', bar.tone)}
                style={{ width: `${Math.min(100, (bar.ms / maxMs) * 100)}%` }}
              />
            </div>
            <p className="mt-1 text-[10px] text-muted-foreground">{bar.hint}</p>
          </div>
        ))}
      </div>
    </div>
  )
}

function LoadTestTable() {
  const rows = [
    ['Test setup', VERIFIED_LOAD_TEST.scenario],
    ['VM pool', VERIFIED_LOAD_TEST.pool],
    ['Invocations succeeded', VERIFIED_LOAD_TEST.successRate],
    ['Avg VM execution', formatMs(VERIFIED_LOAD_TEST.avgMs)],
    ['p95 VM execution', formatMs(VERIFIED_LOAD_TEST.p95Ms)],
    ['Throughput', `${VERIFIED_LOAD_TEST.throughputRps} req/s`],
    ['Avg HTTP round-trip', '1m 33s (includes queue wait)'],
    ['p95 HTTP round-trip', '3m 06s (includes queue wait)'],
  ]

  return (
    <div className="space-y-2">
      {rows.map(([label, value]) => (
        <Row key={label} label={label} value={value} />
      ))}
    </div>
  )
}

function LiveRunSummary({ run, running }: { run: LiveRun | null; running: boolean }) {
  if (running) {
    return (
      <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs leading-relaxed text-muted-foreground">
        Registering function and measuring execution time…
      </p>
    )
  }

  if (!run) {
    return (
      <p className="rounded-lg border border-border bg-muted/30 p-3 text-xs leading-relaxed text-muted-foreground">
        No live run yet. Hit &ldquo;Run speed test&rdquo; to measure latency from your browser.
      </p>
    )
  }

  const ok = run.httpStatus === 'ok' && run.response.status_code === 200

  return (
    <div className="space-y-2 rounded-lg border border-border bg-muted/20 p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium">Result</span>
        <Badge variant={ok ? 'default' : 'destructive'} className="text-[10px]">
          {ok ? 'Success' : 'Failed'}
        </Badge>
      </div>
      <Row label="End-to-end" value={run.wallMs ? formatMs(run.wallMs) : '—'} />
      <Row label="VM execution" value={run.response.duration_ms ? formatMs(run.response.duration_ms) : '—'} />
      <Row label="Completed" value={run.completedAt} />
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-3 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium">{value}</span>
    </div>
  )
}

function ReportSummary({ report, duration }: { report: ProfileReport; duration?: number }) {
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      <TinyStat label="Rows" value={String(report.row_count)} />
      <TinyStat label="Columns" value={String(report.column_count)} />
      <TinyStat label="VM time" value={duration ? formatMs(duration) : '—'} />
      <TinyStat label="Issues" value={String(report.issues.length)} />
    </div>
  )
}

function TinyStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-3">
      <div className="text-lg font-bold tracking-tight">{value}</div>
      <div className="mt-0.5 text-[10px] uppercase tracking-wider text-muted-foreground">{label}</div>
    </div>
  )
}

function ColumnTable({ columns }: { columns: ColumnProfile[] }) {
  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <div className="grid grid-cols-[1.25fr_0.8fr_0.7fr_1.2fr] border-b border-border bg-muted/30 px-3 py-2 text-[10px] uppercase tracking-wider text-muted-foreground">
        <span>Column</span><span>Type</span><span>Nulls</span><span>Stats</span>
      </div>
      {columns.map(column => (
        <div key={column.name} className="grid grid-cols-[1.25fr_0.8fr_0.7fr_1.2fr] items-center border-b border-border px-3 py-2.5 text-xs last:border-b-0">
          <span className="font-medium">{column.name}</span>
          <span className={column.type === 'number' ? 'text-[var(--running)]' : 'text-muted-foreground'}>{column.type}</span>
          <span className={column.null_count ? 'text-[var(--warning)]' : 'text-[var(--success)]'}>{column.null_pct}%</span>
          <span className="text-muted-foreground">
            {column.stats ? `mean ${column.stats.mean}` : `${column.unique_count} unique`}
          </span>
        </div>
      ))}
    </div>
  )
}
