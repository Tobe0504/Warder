"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiFetch } from "@/lib/client-api";

/**
 * Redeems an invitation.
 *
 * The token arrives in the URL fragment, which is why this has to be a client
 * component: a fragment is never sent to a server. That is the point of putting
 * it there: the invitation stays out of this application's access log, out of
 * any proxy in front of it, and out of the Referer header of whatever the
 * invitee clicks next.
 *
 * It is read once on mount and then removed from the address bar, so it does
 * not sit in browser history or get shoulder-read from a URL bar.
 */
export function AcceptInvitationForm() {
  const router = useRouter();

  const [token, setToken] = useState<string | null>(null);
  const [ready, setReady] = useState(false);

  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [done, setDone] = useState<string | null>(null);

  useEffect(() => {
    const fragment = window.location.hash.replace(/^#/, "").trim();
    if (fragment) {
      setToken(fragment);
      // Clear it from the address bar. The value stays in memory for the one
      // request that uses it.
      window.history.replaceState(null, "", window.location.pathname);
    }
    setReady(true);
  }, []);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    if (password !== confirm) {
      setError("Those passwords do not match.");
      return;
    }
    if (!token) {
      setError("This link is missing its invitation. Ask for a new one.");
      return;
    }

    setPending(true);
    try {
      const result = await apiFetch<{ email: string }>("/api/auth/accept-invitation", {
        method: "POST",
        body: { token, name, password },
      });
      setDone(result.email);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "This invitation could not be used.");
    } finally {
      setPending(false);
    }
  }

  if (!ready) {
    return <p className="text-center text-meta text-muted-foreground">Checking the link…</p>;
  }

  if (done) {
    return (
      <div className="text-center">
        <p className="prose-note text-muted-foreground">
          Your account is ready. Sign in as{" "}
          <span className="font-medium text-foreground">{done}</span> with the password you just
          chose.
        </p>
        <Button className="mt-4 w-full" onClick={() => router.push("/login")}>
          Go to sign in
          <ArrowRight />
        </Button>
      </div>
    );
  }

  if (!token) {
    return (
      <div className="text-center">
        <p role="alert" className="prose-note text-muted-foreground">
          This link does not carry an invitation. It may have been shortened, forwarded as plain
          text, or truncated by a chat client, the part after the{" "}
          <span className="font-mono">#</span> matters. Ask whoever invited you for a fresh link.
        </p>
        <Button asChild variant="outline" className="mt-4 w-full">
          <Link href="/login">Sign in instead</Link>
        </Button>
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="accept-name">Your name</Label>
        <Input
          id="accept-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Ada Lovelace"
          autoComplete="name"
          required
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="accept-password">Choose a password</Label>
        <Input
          id="accept-password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          minLength={12}
          required
        />
        <p className="prose-note text-muted-foreground">
          At least 12 characters. Nobody else sees this: not even whoever invited you.
        </p>
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="accept-confirm">Confirm password</Label>
        <Input
          id="accept-confirm"
          type="password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          autoComplete="new-password"
          minLength={12}
          required
        />
      </div>

      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}

      <Button type="submit" className="w-full" disabled={pending}>
        {pending ? "Creating your account…" : "Accept invitation"}
      </Button>
    </form>
  );
}
