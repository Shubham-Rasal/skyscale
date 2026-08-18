"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";
import { authClient } from "@/lib/auth-client";
import { LogoMark } from "@/components/marketing/logo-mark";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default function LoginPage() {
  const router = useRouter();
  const [mode, setMode] = useState<"signin" | "signup">("signin");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [nextPath] = useState(() => {
    if (typeof window === "undefined") return "/lab";
    const next = new URLSearchParams(window.location.search).get("next");
    return next?.startsWith("/") ? next : "/lab";
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
    <main className="relative grid min-h-dvh place-items-center bg-background p-4 sm:p-6">
      <div className="pointer-events-none absolute inset-0 bg-galaxy-subtle opacity-50" />

      <Card className="relative z-10 w-full max-w-md border-border/60 bg-card/80 shadow-none backdrop-blur-sm">
        <CardHeader className="space-y-1">
          <div className="mb-2">
            <LogoMark size={36} />
          </div>
          <CardTitle className="text-xl tracking-tight">
            {mode === "signin" ? "Sign in to Skyscale" : "Create your account"}
          </CardTitle>
          <CardDescription>
            GPU runs are available only to authenticated users.
          </CardDescription>
        </CardHeader>

        <CardContent>
          <form onSubmit={submit} className="space-y-4">
            {mode === "signup" && (
              <div className="space-y-2">
                <Label htmlFor="name">Name</Label>
                <Input
                  id="name"
                  value={name}
                  onChange={event => setName(event.target.value)}
                  placeholder="Ada Lovelace"
                />
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                value={email}
                onChange={event => setEmail(event.target.value)}
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                value={password}
                onChange={event => setPassword(event.target.value)}
                type="password"
                autoComplete={mode === "signin" ? "current-password" : "new-password"}
                minLength={8}
                required
              />
            </div>

            {error && (
              <p className="rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </p>
            )}

            <Button type="submit" className="w-full" disabled={submitting}>
              {submitting ? "Working…" : mode === "signin" ? "Sign in" : "Create account"}
            </Button>

            <Button
              type="button"
              variant="ghost"
              className="w-full"
              onClick={() => {
                setMode(mode === "signin" ? "signup" : "signin");
                setError("");
              }}
            >
              {mode === "signin" ? "Need an account? Sign up" : "Already have an account? Sign in"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
