'use client'

import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { cn } from '@/lib/utils'

type MetricPoint = {
  step: number
  episode_reward: number
  loss: number
  gpu_util: number
  timestamp: number
}

interface RlHeroChartProps {
  data: MetricPoint[]
  className?: string
}

export function RlHeroChart({ data, className }: RlHeroChartProps) {
  if (data.length === 0) {
    return (
      <div className={cn('flex h-48 items-center justify-center rounded-lg border border-border bg-card/40', className)}>
        <p className="text-sm text-muted-foreground">Metrics will appear once training steps are recorded.</p>
      </div>
    )
  }

  return (
    <div className={cn('h-48 rounded-lg border border-border bg-card/40 px-2 py-3', className)}>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 8, right: 12, left: -16, bottom: 0 }}>
          <defs>
            <linearGradient id="rewardFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#34d399" stopOpacity={0.25} />
              <stop offset="100%" stopColor="#34d399" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="var(--border)" strokeDasharray="3 3" vertical={false} />
          <XAxis
            dataKey="step"
            tick={{ fontSize: 10, fill: 'var(--muted-foreground)' }}
            axisLine={false}
            tickLine={false}
          />
          <YAxis
            tick={{ fontSize: 10, fill: 'var(--muted-foreground)' }}
            axisLine={false}
            tickLine={false}
            domain={[0, 'auto']}
          />
          <Tooltip
            contentStyle={{
              background: 'var(--popover)',
              border: '1px solid var(--border)',
              borderRadius: 8,
              fontSize: 11,
            }}
          />
          <Area
            type="monotone"
            dataKey="episode_reward"
            stroke="#34d399"
            strokeWidth={2}
            fill="url(#rewardFill)"
            name="Reward"
          />
          <Line
            type="monotone"
            dataKey="loss"
            stroke="#fbbf24"
            strokeWidth={1.5}
            dot={false}
            name="Loss"
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

interface MiniMetricChartProps {
  title: string
  data: MetricPoint[]
  dataKey: keyof Pick<MetricPoint, 'episode_reward' | 'loss' | 'gpu_util'>
  color: string
  reservedLabel?: string
}

export function MiniMetricChart({ title, data, dataKey, color, reservedLabel = 'Reserved' }: MiniMetricChartProps) {
  const latest = data[data.length - 1]
  const value = latest ? latest[dataKey] : 0

  return (
    <div className="rounded-lg border border-border bg-card/50 p-3">
      <div className="mb-2 flex items-baseline justify-between gap-2">
        <p className="text-xs font-medium text-foreground">{title}</p>
        <span className="font-mono text-xs tabular-nums text-muted-foreground">
          {typeof value === 'number' ? value.toFixed(dataKey === 'loss' ? 3 : 2) : '—'}
        </span>
      </div>
      <div className="h-16">
        {data.length === 0 ? (
          <div className="flex h-full items-center justify-center text-[10px] text-muted-foreground">
            No data
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data} margin={{ top: 2, right: 4, left: -28, bottom: 0 }}>
              <XAxis dataKey="step" hide />
              <YAxis hide domain={['auto', 'auto']} />
              <Line type="monotone" dataKey={dataKey} stroke={color} strokeWidth={1.5} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        )}
      </div>
      <div className="mt-2 flex items-center gap-3 text-[10px] text-muted-foreground">
        <span className="flex items-center gap-1">
          <span className="size-2 rounded-sm" style={{ background: color }} />
          Used
        </span>
        <span className="flex items-center gap-1">
          <span className="size-2 rounded-sm bg-muted-foreground/40" />
          {reservedLabel}
        </span>
      </div>
    </div>
  )
}

export type { MetricPoint }
