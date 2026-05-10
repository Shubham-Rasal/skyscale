'use client'

import { useState } from 'react'
import { Sidebar } from '@/components/Sidebar'
import { SubmitJobDialog } from '@/components/SubmitJobDialog'

const TEMPLATES = [
  {
    name: 'MNIST CNN',
    dialogLabel: 'MNIST CNN',
    description: 'Trains a 3-layer CNN on MNIST using PyTorch + CUDA on an A100. Streams loss, accuracy, and GPU utilisation back to the dashboard every 50 batches.',
    tag: 'GPU · Akash',
    tagColor: 'var(--accent)',
    tagBg: 'var(--accent-dim)',
    tagBorder: 'var(--accent-border)',
    hardware: 'gpu',
    gpuModel: 'a100',
    jobType: 'training_run',
    badge: 'Image Classification',
    icon: '🧠',
  },
  {
    name: 'MNIST CNN on Hugging Face',
    dialogLabel: 'MNIST HF',
    description: 'Runs the same MNIST PyTorch trainer on Hugging Face Jobs using an A10G GPU. Useful for quick smoke tests and GPU sandbox runs without Akash setup.',
    tag: 'GPU · Hugging Face',
    tagColor: 'var(--accent)',
    tagBg: 'var(--accent-dim)',
    tagBorder: 'var(--accent-border)',
    hardware: 'gpu',
    gpuModel: 'a10g',
    jobType: 'training_run',
    badge: 'Image Classification',
    icon: '🤗',
  },
  {
    name: 'CartPole PPO',
    dialogLabel: 'CartPole GPU',
    description: 'Trains a PPO agent on CartPole-v1 with stable-baselines3. Streams live reward and loss curves back to the dashboard.',
    tag: 'GPU · Akash',
    tagColor: 'var(--accent)',
    tagBg: 'var(--accent-dim)',
    tagBorder: 'var(--accent-border)',
    hardware: 'gpu',
    gpuModel: 'rtx3090',
    jobType: 'training_run',
    badge: 'Reinforcement Learning',
    icon: '🎮',
  },
  {
    name: 'CPU FaaS Function',
    dialogLabel: 'CPU FaaS',
    description: 'Run any serverless function on a Firecracker microVM. Fast cold starts, isolated execution, automatic cleanup after completion.',
    tag: 'CPU · Firecracker',
    tagColor: 'var(--text-secondary)',
    tagBg: 'rgba(107,107,107,0.08)',
    tagBorder: 'var(--border)',
    hardware: 'cpu',
    gpuModel: '',
    jobType: 'faas_function',
    badge: 'Serverless',
    icon: '⚡',
  },
  {
    name: 'RL Environment',
    dialogLabel: 'Custom',
    description: 'Host a gymnasium-compatible RL environment on Akash GPU compute, accepting remote agent connections over the network.',
    tag: 'GPU · Akash',
    tagColor: 'var(--accent)',
    tagBg: 'var(--accent-dim)',
    tagBorder: 'var(--accent-border)',
    hardware: 'gpu',
    gpuModel: 'rtx3090',
    jobType: 'rl_env',
    badge: 'Reinforcement Learning',
    icon: '🤖',
  },
]

export default function TemplatesPage() {
  const [selectedLabel, setSelectedLabel] = useState<string | undefined>(undefined)
  const [dialogOpen, setDialogOpen] = useState(false)

  function launch(tpl: typeof TEMPLATES[0]) {
    setSelectedLabel(tpl.dialogLabel)
    setDialogOpen(true)
  }

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={false} />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        {/* Top bar */}
        <div style={{
          height: 56,
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          padding: '0 24px',
          gap: 10,
          flexShrink: 0,
        }}>
          <h1 style={{
            fontFamily: 'var(--font-grotesk), sans-serif',
            fontSize: 15,
            fontWeight: 600,
            color: 'var(--text-primary)',
            letterSpacing: '-0.02em',
          }}>
            Templates
          </h1>
        </div>

        <div style={{ flex: 1, overflowY: 'auto', padding: '28px 24px' }}>
          <div style={{ marginBottom: 24 }}>
            <p style={{
              fontSize: 13,
              color: 'var(--text-secondary)',
              lineHeight: 1.6,
              maxWidth: 520,
            }}>
              Pre-built job templates. Select one to launch on Skyscale with default configuration — no setup required.
            </p>
          </div>

          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))',
            gap: 12,
          }}>
            {TEMPLATES.map(tpl => (
              <TemplateCard key={tpl.name} tpl={tpl} onLaunch={() => launch(tpl)} />
            ))}
          </div>
        </div>
      </div>

      <SubmitJobDialog
        key={selectedLabel ?? 'default'}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSubmitted={() => setDialogOpen(false)}
        initialTemplate={selectedLabel}
      />
    </div>
  )
}

function TemplateCard({ tpl, onLaunch }: { tpl: typeof TEMPLATES[0]; onLaunch: () => void }) {
  return (
    <div
      style={{
        background: 'var(--surface)',
        border: '1px solid var(--border)',
        borderRadius: 12,
        padding: 20,
        display: 'flex',
        flexDirection: 'column',
        gap: 14,
        transition: 'border-color 0.15s, background 0.1s',
        cursor: 'pointer',
      }}
      onMouseEnter={e => {
        const el = e.currentTarget as HTMLElement
        el.style.borderColor = 'var(--border-light)'
        el.style.background = 'var(--bg-hover)'
      }}
      onMouseLeave={e => {
        const el = e.currentTarget as HTMLElement
        el.style.borderColor = 'var(--border)'
        el.style.background = 'var(--surface)'
      }}
      onClick={onLaunch}
    >
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 10 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{
            width: 36,
            height: 36,
            borderRadius: 8,
            background: 'var(--bg-active)',
            border: '1px solid var(--border-light)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontSize: 17,
            flexShrink: 0,
          }}>
            {tpl.icon}
          </div>
          <div>
            <div style={{
              fontSize: 14,
              fontWeight: 600,
              color: 'var(--text-primary)',
              letterSpacing: '-0.01em',
              marginBottom: 2,
            }}>
              {tpl.name}
            </div>
            <span style={{
              fontSize: 10,
              fontWeight: 500,
              padding: '1px 6px',
              borderRadius: 4,
              background: tpl.tagBg,
              color: tpl.tagColor,
              border: `1px solid ${tpl.tagBorder}`,
              whiteSpace: 'nowrap',
            }}>
              {tpl.tag}
            </span>
          </div>
        </div>
      </div>

      {/* Description */}
      <p style={{
        fontSize: 12,
        color: 'var(--text-secondary)',
        lineHeight: 1.65,
        flex: 1,
      }}>
        {tpl.description}
      </p>

      {/* Footer */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingTop: 10,
        borderTop: '1px solid var(--border)',
      }}>
        <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{tpl.badge}</span>
        <button
          onClick={e => { e.stopPropagation(); onLaunch() }}
          style={{
            padding: '5px 14px',
            background: 'var(--bg-active)',
            color: 'var(--text-primary)',
            border: '1px solid var(--border-light)',
            borderRadius: 6,
            fontSize: 12,
            fontWeight: 500,
            cursor: 'pointer',
            fontFamily: 'inherit',
            transition: 'background 0.1s, border-color 0.1s',
          }}
          onMouseEnter={e => {
            const el = e.currentTarget as HTMLButtonElement
            el.style.background = 'var(--accent)'
            el.style.borderColor = 'var(--accent)'
            el.style.color = 'white'
          }}
          onMouseLeave={e => {
            const el = e.currentTarget as HTMLButtonElement
            el.style.background = 'var(--bg-active)'
            el.style.borderColor = 'var(--border-light)'
            el.style.color = 'var(--text-primary)'
          }}
        >
          Use template
        </button>
      </div>
    </div>
  )
}
