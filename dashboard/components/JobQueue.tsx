'use client'

import { Execution, ExecutionContext } from '@/types'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Cpu, Zap } from 'lucide-react'

const STATUS_COLOR: Record<string, string> = {
  pending: 'bg-yellow-500/20 text-yellow-400 border-yellow-500/30',
  running: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
  completed: 'bg-green-500/20 text-green-400 border-green-500/30',
  failed: 'bg-red-500/20 text-red-400 border-red-500/30',
  timeout: 'bg-orange-500/20 text-orange-400 border-orange-500/30',
}

const JOB_TYPE_LABEL: Record<string, string> = {
  training_run: 'Training',
  rl_env: 'RL Env',
  faas_function: 'FaaS',
}

function durationStr(ms: number) {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

function ActiveRow({ ctx }: { ctx: ExecutionContext }) {
  const elapsed = Date.now() - new Date(ctx.StartTime).getTime()
  return (
    <tr className="border-b border-white/5 hover:bg-white/5 transition-colors">
      <td className="px-4 py-3 font-mono text-xs text-slate-400">{ctx.RequestID.slice(0, 8)}</td>
      <td className="px-4 py-3 font-mono text-xs text-slate-300">{ctx.FunctionID.slice(0, 8)}</td>
      <td className="px-4 py-3"><Badge className="bg-blue-500/20 text-blue-400 border-blue-500/30 text-xs">running</Badge></td>
      <td className="px-4 py-3 text-xs text-slate-400">—</td>
      <td className="px-4 py-3 text-xs text-slate-400">{durationStr(elapsed)}</td>
    </tr>
  )
}

function RecentRow({ exec }: { exec: Execution }) {
  const color = STATUS_COLOR[exec.Status] ?? 'bg-slate-500/20 text-slate-400'
  const jobLabel = JOB_TYPE_LABEL[exec.JobType] ?? exec.JobType ?? 'FaaS'
  return (
    <tr className="border-b border-white/5 hover:bg-white/5 transition-colors">
      <td className="px-4 py-3 font-mono text-xs text-slate-400">{exec.ID.slice(0, 8)}</td>
      <td className="px-4 py-3 font-mono text-xs text-slate-300">{exec.FunctionID.slice(0, 8)}</td>
      <td className="px-4 py-3">
        <Badge className={`${color} text-xs`}>{exec.Status}</Badge>
      </td>
      <td className="px-4 py-3">
        <span className="flex items-center gap-1 text-xs text-slate-400">
          {exec.HardwareType === 'gpu'
            ? <Zap size={12} className="text-yellow-400" />
            : <Cpu size={12} className="text-slate-500" />}
          {jobLabel}
        </span>
      </td>
      <td className="px-4 py-3 text-xs text-slate-400">
        {exec.Duration ? durationStr(exec.Duration) : '—'}
      </td>
    </tr>
  )
}

export function JobQueue({ active, recent }: { active: ExecutionContext[]; recent: Execution[] }) {
  const rows = [...active.map(a => ({ type: 'active' as const, data: a })),
                ...recent.map(r => ({ type: 'recent' as const, data: r }))]

  return (
    <Card className="bg-slate-900 border-slate-700/50">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-semibold text-slate-200 flex items-center gap-2">
          Job Queue
          {active.length > 0 && (
            <Badge className="bg-blue-500/20 text-blue-400 border-blue-500/30 text-xs">
              {active.length} running
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/10 text-slate-500 text-xs uppercase tracking-wider">
                <th className="px-4 py-2 text-left">Job ID</th>
                <th className="px-4 py-2 text-left">Function</th>
                <th className="px-4 py-2 text-left">Status</th>
                <th className="px-4 py-2 text-left">Type</th>
                <th className="px-4 py-2 text-left">Duration</th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500 text-xs">
                    No jobs yet — submit one below
                  </td>
                </tr>
              )}
              {rows.map((r, i) =>
                r.type === 'active'
                  ? <ActiveRow key={i} ctx={r.data as ExecutionContext} />
                  : <RecentRow key={i} exec={r.data as Execution} />
              )}
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>
  )
}
