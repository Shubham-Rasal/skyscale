'use client'

import { useRouter } from 'next/navigation'
import { useRealtimeStream } from '@/hooks/useRealtimeStream'
import { DashboardShell } from '@/components/dashboard/dashboard-shell'
import { PageHeader } from '@/components/dashboard/page-header'
import { ContentPanel } from '@/components/dashboard/content-panel'
import { AuthStatus } from '@/components/AuthStatus'
import { authClient } from '@/lib/auth-client'
import { Button } from '@/components/ui/button'

export default function SettingsPage() {
  const router = useRouter()
  const { connected } = useRealtimeStream()
  const { data: session, isPending } = authClient.useSession()

  return (
    <DashboardShell connected={connected}>
      <PageHeader title="Settings" description="Manage your account and workspace." actions={<AuthStatus />} />

      <div className="flex-1 overflow-y-auto p-4 sm:p-6">
        <div className="mx-auto max-w-xl space-y-6">
          <ContentPanel title="Account">
            {isPending ? (
              <p className="text-sm text-muted-foreground">Loading session…</p>
            ) : session ? (
              <div className="space-y-5">
                <div className="grid gap-4">
                  <SettingField label="Email" value={session.user.email} />
                  {session.user.name ? (
                    <SettingField label="Name" value={session.user.name} />
                  ) : null}
                  <SettingField label="User ID" value={session.user.id} mono />
                </div>
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={async () => {
                    await authClient.signOut()
                    router.refresh()
                  }}
                >
                  Sign out
                </Button>
              </div>
            ) : (
              <div className="space-y-4">
                <p className="text-sm leading-relaxed text-muted-foreground">
                  Sign in to sync runs and use GPU launches from this workspace.
                </p>
                <Button size="sm" onClick={() => router.push('/login?next=/settings')}>
                  Sign in
                </Button>
              </div>
            )}
          </ContentPanel>

          <ContentPanel title="Workspace">
            <div className="grid gap-4">
              <SettingField label="Plan" value="Personal" />
              <p className="text-xs leading-relaxed text-muted-foreground">
                Workspace and billing options will appear here as they become available.
              </p>
            </div>
          </ContentPanel>
        </div>
      </div>
    </DashboardShell>
  )
}

function SettingField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <p className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </p>
      <p className={mono ? 'break-all font-mono text-sm text-foreground' : 'text-sm text-foreground'}>
        {value}
      </p>
    </div>
  )
}
