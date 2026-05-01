'use client'

import { TrainingMetric } from '@/types'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend,
} from 'recharts'
import { TrendingUp } from 'lucide-react'

interface Props {
  trainingMetrics: Record<string, TrainingMetric[]>
}

const COLORS = ['#34d399', '#f87171', '#60a5fa', '#fbbf24']

export function TrainingChart({ trainingMetrics }: Props) {
  const jobIDs = Object.keys(trainingMetrics)

  if (jobIDs.length === 0) {
    return (
      <Card className="bg-slate-900 border-slate-700/50">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-semibold text-slate-200 flex items-center gap-2">
            <TrendingUp size={14} className="text-green-400" /> Training Progress
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-xs text-slate-500 text-center py-8">
            No active training runs — submit a CartPole GPU job to see live curves
          </p>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      {jobIDs.map((jobID, idx) => {
        const data = trainingMetrics[jobID]
        const color = COLORS[idx % COLORS.length]
        const latest = data[data.length - 1]

        return (
          <Card key={jobID} className="bg-slate-900 border-slate-700/50">
            <CardHeader className="pb-1">
              <CardTitle className="text-sm font-semibold text-slate-200 flex items-center justify-between">
                <span className="flex items-center gap-2">
                  <TrendingUp size={14} className="text-green-400" />
                  Job {jobID.slice(0, 8)} — CartPole PPO
                </span>
                {latest && (
                  <span className="text-xs text-slate-400 font-normal">
                    step {latest.step.toLocaleString()} · reward {latest.episode_reward.toFixed(1)}
                  </span>
                )}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ResponsiveContainer width="100%" height={180}>
                <LineChart data={data} margin={{ top: 4, right: 8, left: -16, bottom: 0 }}>
                  <XAxis
                    dataKey="step"
                    tick={{ fill: '#64748b', fontSize: 10 }}
                    tickFormatter={v => `${(v / 1000).toFixed(0)}k`}
                  />
                  <YAxis tick={{ fill: '#64748b', fontSize: 10 }} />
                  <Tooltip
                    contentStyle={{ background: '#1e293b', border: '1px solid #334155', borderRadius: 6 }}
                    labelStyle={{ color: '#94a3b8', fontSize: 11 }}
                    itemStyle={{ fontSize: 11 }}
                    labelFormatter={v => `Step ${Number(v).toLocaleString()}`}
                  />
                  <Legend wrapperStyle={{ fontSize: 11, color: '#94a3b8' }} />
                  <Line
                    type="monotone"
                    dataKey="episode_reward"
                    name="Reward"
                    stroke={color}
                    strokeWidth={2}
                    dot={false}
                    isAnimationActive={false}
                  />
                  <Line
                    type="monotone"
                    dataKey="loss"
                    name="Loss"
                    stroke="#f87171"
                    strokeWidth={1.5}
                    dot={false}
                    isAnimationActive={false}
                    strokeDasharray="4 2"
                  />
                </LineChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
