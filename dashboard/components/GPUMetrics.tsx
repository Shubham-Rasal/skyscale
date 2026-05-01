'use client'

import { GPUMetric } from '@/types'
import { Zap } from 'lucide-react'

function Bar({ value, max, color }: { value: number; max: number; color: string }) {
  const pct = Math.min(100, max > 0 ? (value / max) * 100 : 0)
  return (
    <div style={{ width: '100%', background: 'var(--bg-active)', borderRadius: 4, height: 4, overflow: 'hidden' }}>
      <div
        style={{ height: '100%', borderRadius: 4, transition: 'width 0.5s', width: `${pct}%`, background: color }}
      />
    </div>
  )
}

function GPUCard({ m }: { m: GPUMetric }) {
  return (
    <div style={{
      border: '1px solid var(--border)',
      borderRadius: 8,
      padding: 14,
      background: 'var(--bg-active)',
      display: 'flex',
      flexDirection: 'column',
      gap: 12,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Zap size={12} style={{ color: 'var(--accent)' }} />
          <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-primary)' }}>{m.gpu_model || 'GPU'}</span>
        </div>
        <span style={{ fontSize: 10, fontFamily: 'monospace', color: 'var(--text-secondary)' }}>{m.job_id.slice(0, 8)}</span>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--text-secondary)' }}>
          <span>Utilization</span>
          <span style={{ fontWeight: 600, color: 'var(--accent)' }}>{m.utilization_pct}%</span>
        </div>
        <Bar value={m.utilization_pct} max={100} color="var(--accent)" />
      </div>

      {m.memory_total_mb > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--text-secondary)' }}>
            <span>VRAM</span>
            <span style={{ color: 'var(--text-primary)' }}>
              {m.memory_used_mb} / {m.memory_total_mb} MB
            </span>
          </div>
          <Bar value={m.memory_used_mb} max={m.memory_total_mb} color="#8b5cf6" />
        </div>
      )}

      {m.steps_per_sec > 0 && (
        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10, color: 'var(--text-secondary)' }}>
          <span>Steps/sec</span>
          <span style={{ fontWeight: 600, color: '#22c55e' }}>{m.steps_per_sec.toFixed(0)}</span>
        </div>
      )}

      <div style={{ fontSize: 10, color: 'var(--text-secondary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {m.provider.slice(0, 28)}…
      </div>
    </div>
  )
}

export function GPUMetrics({ metrics }: { metrics: GPUMetric[] }) {
  return (
    <div>
      {metrics.length === 0 ? (
        <p style={{ fontSize: 11, color: 'var(--text-secondary)', textAlign: 'center', padding: 16 }}>No active GPU jobs</p>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 12 }}>
          {metrics.map((m, i) => <GPUCard key={i} m={m} />)}
        </div>
      )}
    </div>
  )
}
