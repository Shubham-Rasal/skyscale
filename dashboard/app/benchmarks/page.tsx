'use client'

import { useMemo, useState } from 'react'
import { Sidebar } from '@/components/Sidebar'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { api } from '@/lib/api'

type ColumnProfile = {
  name: string
  type: 'number' | 'string'
  null_count: number
  null_pct: number
  unique_count: number
  stats?: {
    min: number
    max: number
    mean: number
    p50: number
  }
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

const VERIFIED_REPORT: ProfileReport = {
  ok: true,
  row_count: 8,
  column_count: 6,
  quality_score: 87.5,
  columns: [
    { name: 'customer_id', type: 'number', null_count: 0, null_pct: 0, unique_count: 8, stats: { min: 1001, max: 1008, mean: 1004.5, p50: 1005 } },
    { name: 'plan', type: 'string', null_count: 0, null_pct: 0, unique_count: 3 },
    { name: 'monthly_spend', type: 'number', null_count: 1, null_pct: 12.5, unique_count: 6, stats: { min: 9, max: 1200, mean: 346.357, p50: 129.5 } },
    { name: 'events_last_30d', type: 'number', null_count: 1, null_pct: 12.5, unique_count: 7, stats: { min: 4, max: 530, mean: 167.143, p50: 73 } },
    { name: 'churn_risk', type: 'number', null_count: 1, null_pct: 12.5, unique_count: 7, stats: { min: 0.03, max: 0.81, mean: 0.321, p50: 0.2 } },
    { name: 'region', type: 'string', null_count: 3, null_pct: 37.5, unique_count: 3 },
  ],
  issues: [
    { severity: 'medium', column: 'monthly_spend', message: '1 missing values' },
    { severity: 'medium', column: 'events_last_30d', message: '1 missing values' },
    { severity: 'medium', column: 'churn_risk', message: '1 missing values' },
    { severity: 'high', column: 'region', message: '37.5% missing' },
  ],
}

const BENCHMARK_STATS = [
  { label: 'CSV profiles', value: '100/100', tone: 'var(--success)', sub: 'successful Firecracker invocations' },
  { label: 'Function p95', value: '1.68s', tone: 'var(--running)', sub: 'inside the VM, including Python work' },
  { label: 'Quality score', value: '87.5', tone: 'var(--warning)', sub: 'computed from null coverage' },
  { label: 'HTTP failures', value: '0', tone: 'var(--success)', sub: 'k6 threshold passed' },
]

export default function BenchmarksPage() {
  const { data, connected } = useRealtimeStream()
  const [running, setRunning] = useState(false)
  const [liveRun, setLiveRun] = useState<LiveRun | null>(null)
  const [runError, setRunError] = useState<string | null>(null)

  const runResult = liveRun?.response ?? null
  const report = runResult?.output ?? VERIFIED_REPORT
  const headlineStats = useMemo(() => {
    if (!liveRun) return BENCHMARK_STATS

    const ok = liveRun.httpStatus === 'ok' && liveRun.response.status_code === 200
    const duration = liveRun.response.duration_ms
    const output = liveRun.response.output

    return [
      {
        label: 'Live profile',
        value: ok ? '1/1' : '0/1',
        tone: ok ? 'var(--success)' : 'var(--error)',
        sub: `${liveRun.wallMs}ms end-to-end from browser`,
      },
      {
        label: 'VM function time',
        value: duration ? `${duration}ms` : '-',
        tone: 'var(--running)',
        sub: 'reported by the Firecracker daemon',
      },
      {
        label: 'Quality score',
        value: output ? String(output.quality_score) : '-',
        tone: 'var(--warning)',
        sub: output ? `${output.row_count} rows, ${output.column_count} columns profiled` : 'no report returned',
      },
      {
        label: 'Issues found',
        value: output ? String(output.issues.length) : '0',
        tone: output?.issues.some(issue => issue.severity === 'high') ? 'var(--warning)' : 'var(--success)',
        sub: output ? 'missing-value checks from latest run' : 'no data quality issues returned',
      },
    ]
  }, [liveRun])
  const readyWarm = useMemo(
    () => data.vm_pool.filter(vm => (vm.HardwareType === 'cpu' || !vm.HardwareType) && vm.IsWarm && vm.Status === 'ready' && vm.ID !== 'host-vm-test').length,
    [data.vm_pool]
  )
  const busyCPU = useMemo(
    () => data.vm_pool.filter(vm => (vm.HardwareType === 'cpu' || !vm.HardwareType) && vm.Status === 'busy').length,
    [data.vm_pool]
  )

  async function runProfiler() {
    setRunning(true)
    setRunError(null)
    try {
      const name = `csv-profiler-ui-${Date.now()}`
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
        setRunError(result.error_message ?? 'Profiler returned a non-200 status')
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to run profiler'
      setRunError(message)
      setLiveRun({
        functionName: 'csv-profiler-ui',
        httpStatus: 'error',
        wallMs: 0,
        completedAt: new Date().toLocaleTimeString(),
        response: {
          status_code: 500,
          error_message: message,
        },
      })
    } finally {
      setRunning(false)
    }
  }

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={connected} />
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        <TopBar readyWarm={readyWarm} busyCPU={busyCPU} />

        <main style={{ flex: 1, overflowY: 'auto', padding: '28px 24px 40px' }}>
          <section style={{
            border: '1px solid var(--border)',
            borderRadius: 16,
            background: 'linear-gradient(135deg, rgba(124,92,252,0.16), rgba(13,13,13,0.94) 36%, var(--surface))',
            padding: 28,
            marginBottom: 16,
            position: 'relative',
            overflow: 'hidden',
          }}>
            <div style={{ position: 'relative', zIndex: 1, maxWidth: 760 }}>
              <div style={{ fontSize: 11, fontWeight: 700, color: 'var(--accent)', letterSpacing: '0.12em', textTransform: 'uppercase', marginBottom: 14 }}>
                Firecracker benchmark
              </div>
              <h1 style={{ fontFamily: 'var(--font-grotesk), sans-serif', fontSize: 38, lineHeight: 1.04, letterSpacing: '-0.055em', color: 'var(--text-primary)', marginBottom: 14 }}>
                Data quality profiling in isolated microVMs
              </h1>
              <p style={{ fontSize: 14, lineHeight: 1.8, color: 'var(--text-secondary)', maxWidth: 680 }}>
                This benchmark runs a useful serverless workload: parse CSV data, infer schema, count missing values, compute numeric stats, and return a quality report. The verified k6 run used 100 concurrent sync invocations against a two-VM warm pool.
              </p>
            </div>
          </section>

          <section style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: 12, marginBottom: 16 }}>
            {headlineStats.map(stat => (
              <MetricCard key={stat.label} {...stat} />
            ))}
          </section>

          <section style={{ display: 'grid', gridTemplateColumns: 'minmax(420px, 1.15fr) minmax(360px, 0.85fr)', gap: 16, alignItems: 'start' }}>
            <Panel label="Data Quality Report">
              <ReportSummary report={report} duration={runResult?.duration_ms} />
              <ColumnTable columns={report.columns} />
            </Panel>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <Panel label="Run It Live">
                <p style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.7, marginBottom: 14 }}>
                  Registers a fresh Python FaaS function and invokes it on the sample CSV payload. The payload is base64 encoded so multiline data is safe to pass through the current daemon executor.
                </p>
                <button
                  onClick={runProfiler}
                  disabled={running}
                  style={{
                    width: '100%',
                    height: 38,
                    border: '1px solid var(--accent-border)',
                    borderRadius: 8,
                    background: running ? 'var(--bg-active)' : 'var(--accent)',
                    color: running ? 'var(--text-secondary)' : 'white',
                    fontSize: 13,
                    fontWeight: 600,
                    cursor: running ? 'not-allowed' : 'pointer',
                    fontFamily: 'inherit',
                  }}
                >
                  {running ? 'Running profiler...' : 'Run CSV profiler'}
                </button>
                {runError && (
                  <div style={{ marginTop: 12, padding: 10, borderRadius: 8, background: 'var(--error-dim)', border: '1px solid rgba(239,68,68,0.24)', color: 'var(--error)', fontSize: 12, lineHeight: 1.5 }}>
                    {runError}
                  </div>
                )}
              </Panel>

              <Panel label="Latest Live Run">
                <LiveRunRows run={liveRun} running={running} />
              </Panel>

              <Panel label="Verified k6 Result">
                <BenchmarkRows />
              </Panel>

              <Panel label="Sample CSV">
                <pre style={{ whiteSpace: 'pre-wrap', color: 'var(--text-secondary)', fontSize: 11, lineHeight: 1.65, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>
                  {SAMPLE_CSV.trim()}
                </pre>
              </Panel>
            </div>
          </section>
        </main>
      </div>
    </div>
  )
}

function TopBar({ readyWarm, busyCPU }: { readyWarm: number; busyCPU: number }) {
  return (
    <div style={{ height: 56, borderBottom: '1px solid var(--border)', display: 'flex', alignItems: 'center', padding: '0 24px', gap: 10, flexShrink: 0 }}>
      <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 10 }}>
        <h1 style={{ fontFamily: 'var(--font-grotesk), sans-serif', fontSize: 15, fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.02em' }}>
          Benchmarks
        </h1>
        <span style={{ fontSize: 10, fontWeight: 700, padding: '2px 6px', borderRadius: 4, background: 'var(--success-dim)', color: 'var(--success)', letterSpacing: '0.06em', border: '1px solid rgba(34,197,94,0.25)' }}>
          VERIFIED
        </span>
      </div>
      <StatPill label="Warm VMs" value={readyWarm} color="var(--success)" dimColor="var(--success-dim)" />
      <StatPill label="Busy CPU" value={busyCPU} color="var(--running)" dimColor="var(--running-dim)" />
    </div>
  )
}

function Panel({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ background: 'var(--surface)', borderRadius: 12, border: '1px solid var(--border)', overflow: 'hidden' }}>
      <div style={{ padding: '13px 16px', borderBottom: '1px solid var(--border)', fontSize: 11, fontWeight: 700, color: 'var(--text-secondary)', letterSpacing: '0.08em', textTransform: 'uppercase' }}>
        {label}
      </div>
      <div style={{ padding: 16 }}>{children}</div>
    </div>
  )
}

function MetricCard({ label, value, tone, sub }: { label: string; value: string; tone: string; sub: string }) {
  return (
    <div style={{ padding: 16, borderRadius: 12, background: 'var(--surface)', border: '1px solid var(--border)' }}>
      <div style={{ fontSize: 24, fontWeight: 700, color: tone, letterSpacing: '-0.04em', marginBottom: 4 }}>{value}</div>
      <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 4 }}>{label}</div>
      <div style={{ fontSize: 11, color: 'var(--text-secondary)', lineHeight: 1.45 }}>{sub}</div>
    </div>
  )
}

function ReportSummary({ report, duration }: { report: ProfileReport; duration?: number }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10, marginBottom: 16 }}>
      <TinyStat label="Rows" value={String(report.row_count)} />
      <TinyStat label="Columns" value={String(report.column_count)} />
      <TinyStat label="Score" value={String(report.quality_score)} />
      <TinyStat label="VM time" value={duration ? `${duration}ms` : '821ms avg'} />
    </div>
  )
}

function TinyStat({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ padding: 12, background: 'var(--bg-panel)', border: '1px solid var(--border)', borderRadius: 10 }}>
      <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-primary)', letterSpacing: '-0.03em' }}>{value}</div>
      <div style={{ fontSize: 10, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.08em', marginTop: 3 }}>{label}</div>
    </div>
  )
}

function ColumnTable({ columns }: { columns: ColumnProfile[] }) {
  return (
    <div style={{ border: '1px solid var(--border)', borderRadius: 10, overflow: 'hidden' }}>
      <div style={{ display: 'grid', gridTemplateColumns: '1.25fr 0.8fr 0.7fr 1.2fr', padding: '9px 12px', background: 'var(--bg-panel)', borderBottom: '1px solid var(--border)', fontSize: 10, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
        <span>Column</span><span>Type</span><span>Nulls</span><span>Stats</span>
      </div>
      {columns.map(column => (
        <div key={column.name} style={{ display: 'grid', gridTemplateColumns: '1.25fr 0.8fr 0.7fr 1.2fr', padding: '10px 12px', borderBottom: '1px solid var(--border)', alignItems: 'center', fontSize: 12 }}>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>{column.name}</span>
          <span style={{ color: column.type === 'number' ? 'var(--running)' : 'var(--text-secondary)' }}>{column.type}</span>
          <span style={{ color: column.null_count ? 'var(--warning)' : 'var(--success)' }}>{column.null_pct}%</span>
          <span style={{ color: 'var(--text-secondary)' }}>
            {column.stats ? `mean ${column.stats.mean}` : `${column.unique_count} unique`}
          </span>
        </div>
      ))}
    </div>
  )
}

function BenchmarkRows() {
  const rows = [
    ['Scenario', '100 VUs, 100 shared iterations'],
    ['Success rate', '100/100 profiles'],
    ['Function duration', 'avg 821ms, p95 1.68s'],
    ['HTTP duration', 'avg 1m33s, p95 3m06s'],
    ['Throughput', '0.50 req/s'],
  ]
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {rows.map(([label, value]) => (
        <div key={label} style={{ display: 'flex', justifyContent: 'space-between', gap: 12, fontSize: 12 }}>
          <span style={{ color: 'var(--text-secondary)' }}>{label}</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600, textAlign: 'right' }}>{value}</span>
        </div>
      ))}
    </div>
  )
}

function LiveRunRows({ run, running }: { run: LiveRun | null; running: boolean }) {
  if (running) {
    return (
      <div style={{ padding: 12, borderRadius: 10, background: 'var(--bg-panel)', border: '1px solid var(--border)', color: 'var(--text-secondary)', fontSize: 12, lineHeight: 1.6 }}>
        Registering a fresh function and profiling the sample CSV on Firecracker...
      </div>
    )
  }

  if (!run) {
    return (
      <div style={{ padding: 12, borderRadius: 10, background: 'var(--bg-panel)', border: '1px solid var(--border)', color: 'var(--text-secondary)', fontSize: 12, lineHeight: 1.6 }}>
        No live run yet. Click “Run CSV profiler” to replace the headline cards and report with fresh control-plane results.
      </div>
    )
  }

  const output = run.response.output
  const rows = [
    ['Completed', run.completedAt],
    ['Function', run.functionName],
    ['HTTP status', run.httpStatus === 'ok' ? '200 OK' : 'failed'],
    ['Function status', String(run.response.status_code)],
    ['End-to-end', run.wallMs ? `${run.wallMs}ms` : '-'],
    ['VM duration', run.response.duration_ms ? `${run.response.duration_ms}ms` : '-'],
    ['Rows / columns', output ? `${output.row_count} / ${output.column_count}` : '-'],
    ['Issues', output ? `${output.issues.length}` : '-'],
    ['Request ID', run.response.request_id ?? '-'],
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {rows.map(([label, value]) => (
        <div key={label} style={{ display: 'grid', gridTemplateColumns: '100px minmax(0, 1fr)', gap: 12, fontSize: 12 }}>
          <span style={{ color: 'var(--text-secondary)' }}>{label}</span>
          <span style={{ color: 'var(--text-primary)', fontWeight: 600, textAlign: 'right', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={value}>
            {value}
          </span>
        </div>
      ))}
    </div>
  )
}

function StatPill({ label, value, color, dimColor }: { label: string; value: number; color: string; dimColor: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '4px 10px', borderRadius: 6, background: dimColor, border: '1px solid var(--border)', fontSize: 12 }}>
      <span style={{ fontWeight: 700, color }}>{value}</span>
      <span style={{ color: 'var(--text-secondary)' }}>{label}</span>
    </div>
  )
}
