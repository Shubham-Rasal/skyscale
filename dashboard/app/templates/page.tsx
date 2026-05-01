'use client'

import { useState } from 'react'
import { Sidebar } from '@/components/Sidebar'
import { SubmitJobDialog } from '@/components/SubmitJobDialog'

const TEMPLATES = [
  {
    name: 'CartPole PPO',
    description: 'Trains a PPO agent on CartPole-v1 with stable-baselines3. Streams live reward/loss curves back to the dashboard.',
    tag: 'GPU · Akash',
    tagColor: 'var(--accent)',
    tagBg: 'var(--accent-dim)',
    hardware: 'gpu',
    gpuModel: 'rtx3090',
    jobType: 'training_run',
    dockerImage: 'ghcr.io/skyscale/cartpole-trainer:latest',
    input: { total_steps: 50000, report_every: 1000 },
    badge: 'Reinforcement Learning',
  },
  {
    name: 'CPU FaaS Function',
    description: 'Run any serverless function on a Firecracker microVM. Fast cold starts, isolated execution, auto-cleanup.',
    tag: 'CPU · Firecracker',
    tagColor: 'var(--text-secondary)',
    tagBg: 'rgba(107,107,107,0.1)',
    hardware: 'cpu',
    gpuModel: '',
    jobType: 'faas_function',
    dockerImage: '',
    input: {},
    badge: 'Serverless',
  },
  {
    name: 'RL Environment',
    description: 'Host a gymnasium-compatible RL environment on Akash GPU compute, accepting remote agent connections.',
    tag: 'GPU · Akash',
    tagColor: 'var(--accent)',
    tagBg: 'var(--accent-dim)',
    hardware: 'gpu',
    gpuModel: 'rtx3090',
    jobType: 'rl_env',
    dockerImage: '',
    input: {},
    badge: 'Reinforcement Learning',
  },
]

export default function TemplatesPage() {
  const [selected, setSelected] = useState<typeof TEMPLATES[0] | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)

  function launch(tpl: typeof TEMPLATES[0]) {
    setSelected(tpl)
    setDialogOpen(true)
  }

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={false} />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        {/* Top bar */}
        <div style={{
          height: 52,
          borderBottom: '1px solid var(--border)',
          display: 'flex', alignItems: 'center',
          padding: '0 24px', flexShrink: 0,
        }}>
          <h1 style={{
            fontFamily: 'var(--font-grotesk), sans-serif',
            fontSize: 15, fontWeight: 600,
            color: 'var(--text-primary)', letterSpacing: '-0.01em',
          }}>
            Templates
          </h1>
        </div>

        <div style={{ flex: 1, overflowY: 'auto', padding: 24 }}>
          <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 20, maxWidth: 480 }}>
            Pre-built job templates. Select one to launch with default configuration.
          </p>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 12 }}>
            {TEMPLATES.map(tpl => (
              <div
                key={tpl.name}
                style={{
                  background: 'var(--bg-panel)',
                  border: '1px solid var(--border)',
                  borderRadius: 10,
                  padding: 18,
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 10,
                  transition: 'border-color 0.15s',
                  cursor: 'pointer',
                }}
                onMouseEnter={e => (e.currentTarget as HTMLElement).style.borderColor = 'var(--border-light)'}
                onMouseLeave={e => (e.currentTarget as HTMLElement).style.borderColor = 'var(--border)'}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                  <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-primary)' }}>{tpl.name}</span>
                  <span style={{
                    fontSize: 10, fontWeight: 500, padding: '2px 7px', borderRadius: 4,
                    background: tpl.tagBg, color: tpl.tagColor, whiteSpace: 'nowrap', flexShrink: 0,
                  }}>
                    {tpl.tag}
                  </span>
                </div>
                <p style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.6, flex: 1 }}>
                  {tpl.description}
                </p>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{tpl.badge}</span>
                  <button
                    onClick={() => launch(tpl)}
                    style={{
                      padding: '5px 12px',
                      background: 'var(--accent)',
                      color: 'white',
                      border: 'none',
                      borderRadius: 6,
                      fontSize: 12,
                      fontWeight: 500,
                      cursor: 'pointer',
                      fontFamily: 'inherit',
                    }}
                  >
                    Use template
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {selected && (
        <SubmitJobDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          onSubmitted={() => setDialogOpen(false)}
        />
      )}
    </div>
  )
}
