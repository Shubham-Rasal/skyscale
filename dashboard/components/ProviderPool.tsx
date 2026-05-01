'use client'

import { VM } from '@/types'
import { Cpu, Zap, Server } from 'lucide-react'

function StatusDot({ status }: { status: string }) {
  const color =
    status === 'ready' ? '#22c55e' :
    status === 'busy'  ? '#3b82f6' :
    'var(--text-secondary)'
  return (
    <span style={{
      display: 'inline-block',
      width: 6,
      height: 6,
      borderRadius: '50%',
      background: color,
      animation: status === 'busy' ? 'pulse 1.5s infinite' : 'none',
    }} />
  )
}

function VMCard({ vm }: { vm: VM }) {
  const isGPU = vm.HardwareType === 'gpu'
  const isAkash = !!vm.AkashDeploymentID

  return (
    <div style={{
      border: '1px solid var(--border)',
      borderRadius: 8,
      padding: 12,
      background: isGPU ? 'var(--bg-active)' : 'transparent',
      display: 'flex',
      flexDirection: 'column',
      gap: 8,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          {isGPU ? <Zap size={12} style={{ color: 'var(--accent)' }} /> : <Cpu size={12} style={{ color: 'var(--text-secondary)' }} />}
          <span style={{ fontSize: 11, fontFamily: 'monospace', color: 'var(--text-primary)' }}>{vm.ID.slice(0, 10)}</span>
        </div>
        <StatusDot status={vm.Status} />
      </div>

      {isAkash && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 10, color: 'var(--accent)', marginBottom: 4 }}>
          <Server size={10} />
          <span>Akash Network</span>
        </div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 4, fontSize: 10, color: 'var(--text-secondary)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>Status</span>
          <span style={{ color: 'var(--text-primary)', textTransform: 'capitalize' }}>{vm.Status}</span>
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <span>IP</span>
          <span style={{ fontFamily: 'monospace', color: 'var(--text-primary)' }}>{vm.IP || '—'}</span>
        </div>
        {isGPU ? (
          <>
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>GPU</span>
              <span style={{ color: 'var(--accent)' }}>{vm.GPUModel || 'unknown'}</span>
            </div>
            {vm.VRAMmb > 0 && (
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span>VRAM</span>
                <span style={{ color: 'var(--text-primary)' }}>{vm.VRAMmb} MB</span>
              </div>
            )}
            {isAkash && vm.ProviderAddr && (
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span>Provider</span>
                <span style={{ fontFamily: 'monospace', color: 'var(--text-primary)' }}>{vm.ProviderAddr.slice(0, 12)}…</span>
              </div>
            )}
          </>
        ) : (
          <>
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>Memory</span>
              <span style={{ color: 'var(--text-primary)' }}>{vm.Memory} MB</span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>CPU</span>
              <span style={{ color: 'var(--text-primary)' }}>{vm.CPU} vCPU</span>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

export function ProviderPool({ vms }: { vms: VM[] }) {
  const local = vms.filter(v => !v.AkashDeploymentID)
  const akash = vms.filter(v => !!v.AkashDeploymentID)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {local.length > 0 && (
        <div>
          <p style={{ fontSize: 10, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 10, display: 'flex', alignItems: 'center', gap: 4 }}>
            <Cpu size={10} /> Local — Firecracker
          </p>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))', gap: 8 }}>
            {local.map(v => <VMCard key={v.ID} vm={v} />)}
          </div>
        </div>
      )}
      {akash.length > 0 && (
        <div>
          <p style={{ fontSize: 10, color: 'var(--accent)', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 10, display: 'flex', alignItems: 'center', gap: 4 }}>
            <Zap size={10} /> Akash GPU
          </p>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))', gap: 8 }}>
            {akash.map(v => <VMCard key={v.ID} vm={v} />)}
          </div>
        </div>
      )}
      {vms.length === 0 && (
        <p style={{ fontSize: 11, color: 'var(--text-secondary)', textAlign: 'center', padding: 16 }}>No providers online</p>
      )}
    </div>
  )
}
