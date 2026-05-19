"use client";

import { useRouter } from "next/navigation";
import { authClient } from "@/lib/auth-client";

export function AuthStatus() {
  const router = useRouter();
  const { data: session, isPending } = authClient.useSession();

  if (isPending) {
    return (
      <span style={{ color: "var(--text-secondary)", fontSize: 12 }}>
        Checking session...
      </span>
    );
  }

  if (!session) {
    return (
      <button
        onClick={() => router.push("/login?next=/")}
        style={authButtonStyle}
      >
        Sign in
      </button>
    );
  }

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <span style={{ color: "var(--text-secondary)", fontSize: 12 }}>
        {session.user.email}
      </span>
      <button
        onClick={async () => {
          await authClient.signOut();
          router.refresh();
        }}
        style={authButtonStyle}
      >
        Sign out
      </button>
    </div>
  );
}

const authButtonStyle: React.CSSProperties = {
  padding: "5px 12px",
  background: "var(--bg-active)",
  border: "1px solid var(--border)",
  borderRadius: 7,
  color: "var(--text-primary)",
  fontSize: 12,
  fontWeight: 500,
  cursor: "pointer",
  fontFamily: "inherit",
};
