'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'

const NAV = [
  {
    section: 'Lab',
    items: [
      { label: 'Training', href: '/', icon: TrainIcon },
      { label: 'Templates', href: '/templates', icon: GridIcon },
    ],
  },
  {
    section: 'Compute',
    items: [
      { label: 'On-Demand GPUs', href: '/gpus', icon: GpuIcon },
    ],
  },
  {
    section: 'Account',
    items: [
      { label: 'Keys & Secrets', href: '/keys', icon: KeyIcon },
      { label: 'Settings', href: '/settings', icon: SettingsIcon },
    ],
  },
]

export function Sidebar({ connected }: { connected: boolean }) {
  const pathname = usePathname()

  return (
    <aside style={{
      width: 240,
      minWidth: 240,
      background: 'var(--bg-panel)',
      borderRight: '1px solid var(--border)',
      display: 'flex',
      flexDirection: 'column',
      height: '100%',
      userSelect: 'none',
    }}>
      {/* Logo */}
      <div style={{ padding: '18px 16px 16px', borderBottom: '1px solid var(--border)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
          <div style={{
            width: 28, height: 28,
            background: 'linear-gradient(135deg, #7c5cfc 0%, #5b3fd4 100%)',
            borderRadius: 7,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            flexShrink: 0,
            boxShadow: '0 2px 8px rgba(124,92,252,0.4)',
          }}>
            <svg width="15" height="15" viewBox="0 0 14 14" fill="none">
              <path d="M7 1L13 4.5V9.5L7 13L1 9.5V4.5L7 1Z" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
            </svg>
          </div>
          <div>
            <span style={{
              fontFamily: 'var(--font-grotesk), sans-serif',
              fontWeight: 600,
              fontSize: 14,
              color: 'var(--text-primary)',
              letterSpacing: '-0.02em',
              display: 'block',
            }}>
              Skyscale
            </span>
          </div>
        </div>
      </div>

      {/* Nav */}
      <nav style={{ flex: 1, overflowY: 'auto', padding: '10px 8px' }}>
        {NAV.map(group => (
          <div key={group.section} style={{ marginBottom: 8 }}>
            <div style={{
              padding: '6px 8px 4px',
              fontSize: 10,
              fontWeight: 600,
              color: 'var(--text-muted)',
              letterSpacing: '0.1em',
              textTransform: 'uppercase',
            }}>
              {group.section}
            </div>
            {group.items.map(item => {
              const active = pathname === item.href
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 9,
                    padding: '7px 8px',
                    marginBottom: 1,
                    borderRadius: 7,
                    color: active ? 'var(--text-primary)' : 'var(--text-secondary)',
                    background: active ? 'var(--bg-active)' : 'transparent',
                    fontSize: 13,
                    fontWeight: active ? 500 : 400,
                    transition: 'background 0.1s, color 0.1s',
                    position: 'relative',
                    borderLeft: active ? '2px solid var(--accent)' : '2px solid transparent',
                  }}
                  onMouseEnter={e => {
                    if (!active) {
                      const el = e.currentTarget as HTMLElement
                      el.style.background = 'var(--bg-hover)'
                      el.style.color = 'var(--text-primary)'
                    }
                  }}
                  onMouseLeave={e => {
                    if (!active) {
                      const el = e.currentTarget as HTMLElement
                      el.style.background = 'transparent'
                      el.style.color = 'var(--text-secondary)'
                    }
                  }}
                >
                  <item.icon size={14} color={active ? 'var(--accent)' : 'currentColor'} />
                  <span>{item.label}</span>
                </Link>
              )
            })}
          </div>
        ))}
      </nav>

      {/* User section */}
      <div style={{ borderTop: '1px solid var(--border)', padding: '12px 12px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
          <div style={{
            width: 30, height: 30,
            borderRadius: '50%',
            background: 'linear-gradient(135deg, #7c5cfc 0%, #3b82f6 100%)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontSize: 11, fontWeight: 600, color: 'white', flexShrink: 0,
            letterSpacing: '0.02em',
          }}>SR</div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{
              fontSize: 12,
              fontWeight: 500,
              color: 'var(--text-primary)',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}>
              Shubham Rasal
            </div>
            <div style={{ fontSize: 11, color: 'var(--text-secondary)', marginTop: 1 }}>
              Personal
            </div>
          </div>
          <div style={{
            width: 6, height: 6, borderRadius: '50%',
            background: connected ? 'var(--success)' : 'var(--text-muted)',
            flexShrink: 0,
          }} />
        </div>
      </div>
    </aside>
  )
}

// Icons
function TrainIcon({ size = 14, color = 'currentColor' }: { size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 14 14" fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="1,11 5,6 8,8 13,3" />
      <polyline points="10,3 13,3 13,6" />
    </svg>
  )
}
function GridIcon({ size = 14, color = 'currentColor' }: { size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 14 14" fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="1" y="1" width="5" height="5" rx="1.5"/>
      <rect x="8" y="1" width="5" height="5" rx="1.5"/>
      <rect x="1" y="8" width="5" height="5" rx="1.5"/>
      <rect x="8" y="8" width="5" height="5" rx="1.5"/>
    </svg>
  )
}
function GpuIcon({ size = 14, color = 'currentColor' }: { size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 14 14" fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="3" width="10" height="8" rx="1.5"/>
      <path d="M5 3V1M7 3V1M9 3V1M5 11v2M7 11v2M9 11v2"/>
    </svg>
  )
}
function KeyIcon({ size = 14, color = 'currentColor' }: { size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 14 14" fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="5.5" cy="5.5" r="3.5"/>
      <path d="M8.5 8.5l4 4M10 7l1.5 1.5"/>
    </svg>
  )
}
function SettingsIcon({ size = 14, color = 'currentColor' }: { size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 14 14" fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="7" cy="7" r="2"/>
      <path d="M7 1v2M7 11v2M1 7h2M11 7h2M2.93 2.93l1.41 1.41M9.66 9.66l1.41 1.41M2.93 11.07l1.41-1.41M9.66 4.34l1.41-1.41"/>
    </svg>
  )
}
