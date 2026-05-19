'use client'

import { useRouter } from 'next/navigation'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { Sidebar } from '@/components/Sidebar'
import { AuthStatus } from '@/components/AuthStatus'
import { authClient } from '@/lib/auth-client'

export default function SettingsPage() {
  const router = useRouter()
  const { connected } = useRealtimeStream()
  const { data: session, isPending } = authClient.useSession()

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--bg)' }}>
      <Sidebar connected={connected} />

      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', minWidth: 0 }}>
        <div style={{
          height: 56,
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          padding: '0 24px',
          gap: 12,
          flexShrink: 0,
        }}>
          <div style={{ flex: 1 }}>
            <h1 style={{
              fontFamily: 'var(--font-grotesk), sans-serif',
              fontSize: 15,
              fontWeight: 600,
              color: 'var(--text-primary)',
              letterSpacing: '-0.02em',
            }}>
              Settings
            </h1>
          </div>
          <AuthStatus />
        </div>

        <div style={{ flex: 1, overflowY: 'auto', padding: '28px 24px' }}>
          <p style={{
            fontSize: 13,
            color: 'var(--text-secondary)',
            lineHeight: 1.6,
            maxWidth: 520,
            marginBottom: 24,
          }}>
            Manage your account and how you appear in the dashboard.
          </p>

          <Panel label="Account">
            {isPending ? (
              <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>Loading session…</div>
            ) : session ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <div style={{ display: 'grid', gap: 14 }}>
                  <Field label="Email" value={session.user.email} />
                  {session.user.name ? (
                    <Field label="Name" value={session.user.name} />
                  ) : null}
                  <Field label="User ID" value={session.user.id} mono />
                </div>
                <button
                  type="button"
                  onClick={async () => {
                    await authClient.signOut()
                    router.refresh()
                  }}
                  style={{
                    alignSelf: 'flex-start',
                    padding: '7px 14px',
                    background: 'var(--error-dim)',
                    color: 'var(--error)',
                    border: '1px solid rgba(239, 68, 68, 0.35)',
                    borderRadius: 7,
                    fontSize: 13,
                    fontWeight: 500,
                    cursor: 'pointer',
                    fontFamily: 'inherit',
                  }}
                >
                  Sign out
                </button>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <p style={{ fontSize: 13, color: 'var(--text-secondary)', lineHeight: 1.55 }}>
                  Sign in to sync runs and use GPU launches from this workspace.
                </p>
                <button
                  type="button"
                  onClick={() => router.push('/login?next=/settings')}
                  style={{
                    alignSelf: 'flex-start',
                    padding: '7px 14px',
                    background: 'var(--accent)',
                    color: 'white',
                    border: 'none',
                    borderRadius: 7,
                    fontSize: 13,
                    fontWeight: 500,
                    cursor: 'pointer',
                    fontFamily: 'inherit',
                  }}
                >
                  Sign in
                </button>
              </div>
            )}
          </Panel>

          <div style={{ height: 16 }} />

          <Panel label="Workspace">
            <div style={{ display: 'grid', gap: 14 }}>
              <Field label="Plan" value="Personal" />
              <p style={{ fontSize: 12, color: 'var(--text-muted)', lineHeight: 1.55 }}>
                Workspace and billing options will appear here as they become available.
              </p>
            </div>
          </Panel>
        </div>
      </div>
    </div>
  )
}

function Panel({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{
      background: 'var(--surface)',
      borderRadius: 12,
      border: '1px solid var(--border)',
      overflow: 'hidden',
      maxWidth: 560,
    }}>
      <div style={{
        padding: '14px 20px',
        borderBottom: '1px solid var(--border)',
        fontSize: 11,
        fontWeight: 600,
        color: 'var(--text-secondary)',
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
      }}>
        {label}
      </div>
      <div style={{ padding: '20px' }}>
        {children}
      </div>
    </div>
  )
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <div style={{
        fontSize: 10,
        fontWeight: 600,
        color: 'var(--text-muted)',
        letterSpacing: '0.06em',
        textTransform: 'uppercase',
        marginBottom: 6,
      }}>
        {label}
      </div>
      <div style={{
        fontSize: 13,
        color: 'var(--text-primary)',
        fontFamily: mono ? 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' : 'inherit',
        wordBreak: 'break-all',
      }}>
        {value}
      </div>
    </div>
  )
}
