"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";
import { ApiError, apiFetch } from "@/lib/client-api";

export function RedeemRecoveryForm() {
  const router = useRouter();

  const [token, setToken] = useState<string | null>(null);
  const [ready, setReady] = useState(false);
  const [newPassword, setNewPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [done, setDone] = useState<string | null>(null);
  const [passkeysRemoved, setPasskeysRemoved] = useState(0);

  useEffect(() => {
    const fragment = window.location.hash.replace(/^#/, "").trim();
    if (fragment) {
      setToken(fragment);
      // Cleared from the address bar so it does not sit in history or get
      // read over a shoulder. The value stays in memory for the one request
      // that uses it.
      window.history.replaceState(null, "", window.location.pathname);
    }
    setReady(true);
  }, []);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setFieldErrors({});

    if (!token) {
      setError("This link is missing its token. Ask for a new one.");
      return;
    }

    setPending(true);
    try {
      const result = await apiFetch<{ email: string; passkeysRemoved?: number }>(
        "/api/auth/password/recover",
        { method: "POST", body: { token, newPassword } },
      );
      setDone(result.email);
      setPasskeysRemoved(result.passkeysRemoved ?? 0);
    } catch (caught) {
      if (caught instanceof ApiError && caught.details) {
        setFieldErrors(caught.details);
      }
      setError(
        caught instanceof Error
          ? caught.message
          : "This link could not be used.",
      );
      setPending(false);
    } finally {
      setNewPassword("");
    }
  }

  if (!ready) {
    return (
      <p className="text-center text-meta text-muted-foreground">
        Checking the link…
      </p>
    );
  }

  if (done) {
    return (
      <div className="space-y-4 text-center">
        <p className="prose-note text-muted-foreground">
          Your password is set. Sign in as{" "}
          <span className="font-medium text-foreground">{done}</span>.
        </p>
        {passkeysRemoved > 0 && (
          <p className="prose-note text-muted-foreground">
            Your old passkeys were removed. Add new ones, and issue fresh recovery codes, from
            Settings once you&apos;re in.
          </p>
        )}
        <Button className="w-full" onClick={() => router.replace("/login")}>
          Sign in
          <ArrowRight />
        </Button>
      </div>
    );
  }

  if (!token) {
    return (
      <p role="alert" className="prose-note text-center text-muted-foreground">
        This link does not carry a token. It may have been shortened or
        truncated: the part after the <span className="font-mono">#</span>{" "}
        matters. Ask whoever issued it for a fresh one.
      </p>
    );
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3.5">
      <Input
        id="newPassword"
        label="New password"
        type="password"
        value={newPassword}
        onChange={setNewPassword}
        error={fieldErrors.newPassword}
        autoFocus
        required
      />
      <p className="prose-note text-muted-foreground">At least 12 characters.</p>

      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}

      <Button type="submit" size="lg" className="w-full" loading={pending}>
        Set password
      </Button>
    </form>
  );
}
