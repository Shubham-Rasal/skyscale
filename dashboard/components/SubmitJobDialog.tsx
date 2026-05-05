'use client'

import { useState } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api } from '@/lib/api'

const TEMPLATES = [
  {
    label: 'MNIST CNN',
    defaults: {
      functionName: 'mnist-trainer',
      jobType: 'training_run',
      hardwareType: 'gpu',
      gpuModel: 'a100',
      dockerImage: 'ghcr.io/shubham-rasal/skyscale-mnist:v1',
      controlPlaneURL: 'http://n8n.maximalstudio.in:8080',
      input: JSON.stringify({ epochs: 3, batch_size: 512 }, null, 2),
    },
  },
  {
    label: 'CartPole GPU',
    defaults: {
      functionName: 'cartpole-trainer',
      jobType: 'training_run',
      hardwareType: 'gpu',
      gpuModel: 'a100',
      dockerImage: 'ghcr.io/shubham-rasal/skyscale/skyscale-trainer:latest',
      controlPlaneURL: 'http://n8n.maximalstudio.in:8080',
      input: JSON.stringify({ total_steps: 50000, report_every: 1000 }, null, 2),
    },
  },
  {
    label: 'CPU FaaS',
    defaults: {
      functionName: '',
      jobType: 'faas_function',
      hardwareType: 'cpu',
      gpuModel: '',
      dockerImage: '',
      controlPlaneURL: '',
      input: '{}',
    },
  },
  {
    label: 'Custom',
    defaults: {
      functionName: '',
      jobType: 'faas_function',
      hardwareType: 'cpu',
      gpuModel: '',
      dockerImage: '',
      controlPlaneURL: '',
      input: '{}',
    },
  },
]

interface Props {
  open?: boolean
  onOpenChange?: (o: boolean) => void
  onSubmitted?: () => void
  trigger?: React.ReactNode
  initialTemplate?: string // label of a TEMPLATES entry to pre-select
}

const field: React.CSSProperties = {
  width: '100%',
  height: 34,
  background: '#141414',
  border: '1px solid #2a2a2a',
  borderRadius: 6,
  padding: '0 10px',
  color: '#e8e8e8',
  fontSize: 13,
  fontFamily: 'inherit',
  outline: 'none',
}

const label: React.CSSProperties = {
  fontSize: 11,
  color: '#6b6b6b',
  display: 'block',
  marginBottom: 5,
  fontWeight: 500,
}

export function SubmitJobDialog({ open, onOpenChange, onSubmitted, trigger, initialTemplate }: Props) {
  const initIdx = initialTemplate ? Math.max(0, TEMPLATES.findIndex(t => t.label === initialTemplate)) : 0
  const [tplIdx, setTplIdx] = useState(initIdx)
  const [fields, setFields] = useState(TEMPLATES[initIdx].defaults)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  function applyTemplate(idx: number) {
    setTplIdx(idx)
    setFields(TEMPLATES[idx].defaults)
    setError('')
  }

  function set(k: string, v: string) {
    setFields(f => ({ ...f, [k]: v }))
  }

  async function submit() {
    setError('')
    let input: Record<string, unknown> = {}
    try { input = JSON.parse(fields.input) } catch {
      setError('Input must be valid JSON')
      return
    }
    if (!fields.functionName) { setError('Function name is required'); return }
    setSubmitting(true)
    try {
      await api.invoke(fields.functionName, {
        input,
        sync: false,
        job_type: fields.jobType,
        hardware_type: fields.hardwareType,
        gpu_model: fields.gpuModel || undefined,
        docker_image: fields.dockerImage || undefined,
        control_plane_url: fields.controlPlaneURL || undefined,
      })
      onSubmitted?.()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Request failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger}
      <DialogContent className="submit-dialog">
        <DialogHeader>
          <DialogTitle className="submit-dialog-title">New Training Run</DialogTitle>
        </DialogHeader>

        {/* Template tabs */}
        <div style={{ display: 'flex', gap: 4, marginBottom: 2 }}>
          {TEMPLATES.map((t, i) => (
            <button
              key={i}
              onClick={() => applyTemplate(i)}
              style={{
                padding: '4px 11px',
                borderRadius: 5,
                border: tplIdx === i ? '1px solid rgba(124,92,252,0.5)' : '1px solid #222',
                background: tplIdx === i ? 'rgba(124,92,252,0.1)' : 'transparent',
                color: tplIdx === i ? '#a78bfa' : '#6b6b6b',
                fontSize: 12,
                fontWeight: tplIdx === i ? 500 : 400,
                cursor: 'pointer',
                fontFamily: 'inherit',
                transition: 'all 0.1s',
              }}
            >
              {t.label}
            </button>
          ))}
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <div>
            <span style={label}>Function Name *</span>
            <input
              value={fields.functionName}
              onChange={e => set('functionName', e.target.value)}
              style={field}
              placeholder="my-trainer"
            />
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
            <div>
              <span style={label}>Job Type</span>
              <select
                value={fields.jobType}
                onChange={e => set('jobType', e.target.value)}
                style={{ ...field, cursor: 'pointer', appearance: 'none', backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' fill='none'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%236b6b6b' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")`, backgroundRepeat: 'no-repeat', backgroundPosition: 'right 10px center', paddingRight: 28 }}
              >
                <option value="faas_function">FaaS Function</option>
                <option value="training_run">Training Run</option>
                <option value="rl_env">RL Environment</option>
              </select>
            </div>
            <div>
              <span style={label}>Hardware</span>
              <select
                value={fields.hardwareType}
                onChange={e => set('hardwareType', e.target.value)}
                style={{ ...field, cursor: 'pointer', appearance: 'none', backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' fill='none'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%236b6b6b' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")`, backgroundRepeat: 'no-repeat', backgroundPosition: 'right 10px center', paddingRight: 28 }}
              >
                <option value="cpu">CPU (Firecracker)</option>
                <option value="gpu">GPU (Akash)</option>
              </select>
            </div>
          </div>

          {fields.hardwareType === 'gpu' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
              <div>
                <span style={label}>GPU Model</span>
                <select
                  value={fields.gpuModel}
                  onChange={e => set('gpuModel', e.target.value)}
                  style={{ ...field, cursor: 'pointer', appearance: 'none', backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' fill='none'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%236b6b6b' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")`, backgroundRepeat: 'no-repeat', backgroundPosition: 'right 10px center', paddingRight: 28 }}
                >
                  <option value="a100">A100 80GB (SXM4) — 22 free</option>
                  <option value="h100">H100 80GB (SXM5) — 18 free</option>
                  <option value="h200">H200 141GB (SXM5) — 20 free</option>
                  <option value="rtx4090">RTX 4090 24GB — 7 free</option>
                  <option value="rtx3090">RTX 3090 24GB — 7 free</option>
                  <option value="rtx3060">RTX 3060 12GB — 3 free</option>
                  <option value="rtx6000">RTX 6000 24GB — 4 free</option>
                  <option value="t4">T4 16GB — 1 free</option>
                </select>
              </div>
              <div>
                <span style={label}>Docker Image</span>
                <input value={fields.dockerImage} onChange={e => set('dockerImage', e.target.value)} style={field} placeholder="ghcr.io/…" />
              </div>
            </div>
          )}

          <div>
            <span style={label}>Input (JSON)</span>
            <textarea
              value={fields.input}
              onChange={e => set('input', e.target.value)}
              style={{
                ...field,
                height: 'auto',
                minHeight: 80,
                padding: '8px 10px',
                resize: 'vertical',
                fontFamily: 'monospace',
                fontSize: 12,
                lineHeight: 1.5,
              }}
              placeholder="{}"
            />
          </div>

          {error && <p style={{ fontSize: 12, color: '#ef4444', marginTop: -4 }}>{error}</p>}

          <button
            onClick={submit}
            disabled={submitting}
            style={{
              width: '100%',
              height: 36,
              background: submitting ? 'rgba(124,92,252,0.5)' : '#7c5cfc',
              color: 'white',
              border: 'none',
              borderRadius: 7,
              fontSize: 13,
              fontWeight: 500,
              cursor: submitting ? 'not-allowed' : 'pointer',
              fontFamily: 'inherit',
              transition: 'background 0.1s',
              marginTop: 2,
            }}
            onMouseEnter={e => { if (!submitting) (e.currentTarget as HTMLButtonElement).style.background = '#6d4ef0' }}
            onMouseLeave={e => { if (!submitting) (e.currentTarget as HTMLButtonElement).style.background = '#7c5cfc' }}
          >
            {submitting ? 'Submitting…' : 'Submit Run'}
          </button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
