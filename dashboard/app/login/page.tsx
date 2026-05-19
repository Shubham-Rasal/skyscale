"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";
import { authClient } from "@/lib/auth-client";

export default function LoginPage() {
  const router = useRouter();
  const [mode, setMode] = useState<"signin" | "signup">("signin");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [nextPath] = useState(() => {
    if (typeof window === "undefined") return "/";
    const next = new URLSearchParams(window.location.search).get("next");
    return next?.startsWith("/") ? next : "/";
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setSubmitting(true);

    const result = mode === "signin"
      ? await authClient.signIn.email({
          email,
          password,
          callbackURL: nextPath,
          rememberMe: true,
        })
      : await authClient.signUp.email({
          name: name || email,
          email,
          password,
          callbackURL: nextPath,
        });

    setSubmitting(false);

    if (result.error) {
      setError(result.error.message || "Authentication failed");
      return;
    }

    router.push(nextPath);
    router.refresh();
  }

  return (
    <main style={{
      minHeight: "100vh",
      display: "grid",
      placeItems: "center",
      background: "radial-gradient(circle at top, rgba(124,92,252,0.16), transparent 32%), var(--bg)",
      padding: 24,
    }}>
      <form
        onSubmit={submit}
        style={{
          width: "100%",
          maxWidth: 390,
          background: "var(--bg-panel)",
          border: "1px solid var(--border-light)",
          borderRadius: 16,
          padding: 28,
          boxShadow: "0 24px 70px rgba(0,0,0,0.45)",
        }}
      >
        <div style={{ marginBottom: 24 }}>
          <div style={{
            fontFamily: "var(--font-grotesk), sans-serif",
            fontSize: 22,
            fontWeight: 600,
            letterSpacing: "-0.04em",
            marginBottom: 8,
          }}>
            {mode === "signin" ? "Sign in to Skyscale" : "Create your Skyscale account"}
          </div>
          <p style={{ color: "var(--text-secondary)", lineHeight: 1.6 }}>
            GPU runs are available only to authenticated users.
          </p>
        </div>

        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {mode === "signup" && (
            <label style={labelStyle}>
              Name
              <input
                value={name}
                onChange={event => setName(event.target.value)}
                style={inputStyle}
                placeholder="Ada Lovelace"
              />
            </label>
          )}

          <label style={labelStyle}>
            Email
            <input
              value={email}
              onChange={event => setEmail(event.target.value)}
              style={inputStyle}
              type="email"
              autoComplete="email"
              placeholder="you@example.com"
              required
            />
          </label>

          <label style={labelStyle}>
            Password
            <input
              value={password}
              onChange={event => setPassword(event.target.value)}
              style={inputStyle}
              type="password"
              autoComplete={mode === "signin" ? "current-password" : "new-password"}
              minLength={8}
              required
            />
          </label>
        </div>

        {error && (
          <p style={{
            color: "var(--error)",
            background: "var(--error-dim)",
            border: "1px solid rgba(239,68,68,0.25)",
            borderRadius: 8,
            padding: "9px 10px",
            marginTop: 14,
          }}>
            {error}
          </p>
        )}

        <button
          disabled={submitting}
          style={{
            width: "100%",
            height: 40,
            marginTop: 18,
            background: submitting ? "rgba(124,92,252,0.5)" : "var(--accent)",
            color: "white",
            border: "none",
            borderRadius: 9,
            fontFamily: "inherit",
            fontWeight: 600,
            cursor: submitting ? "not-allowed" : "pointer",
          }}
        >
          {submitting ? "Working..." : mode === "signin" ? "Sign in" : "Create account"}
        </button>

        <button
          type="button"
          onClick={() => {
            setMode(mode === "signin" ? "signup" : "signin");
            setError("");
          }}
          style={{
            width: "100%",
            marginTop: 14,
            background: "transparent",
            color: "var(--text-secondary)",
            border: "none",
            cursor: "pointer",
            fontFamily: "inherit",
          }}
        >
          {mode === "signin" ? "Need an account? Sign up" : "Already have an account? Sign in"}
        </button>
      </form>
    </main>
  );
}

const labelStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
  color: "var(--text-secondary)",
  fontSize: 12,
  fontWeight: 500,
};

const inputStyle: React.CSSProperties = {
  height: 38,
  background: "var(--surface)",
  border: "1px solid var(--border)",
  borderRadius: 8,
  color: "var(--text-primary)",
  outline: "none",
  padding: "0 11px",
  fontFamily: "inherit",
};
