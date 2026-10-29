"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowRight, Fingerprint } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";
import {
  describePasskeyError,
  getCredential,
  passkeysSupported,
} from "@/lib/webauthn";

type ErrorPayload = { error?: { code?: string; message?: string } } | null;

/**
 * Sign-in.
 *
 * The password lives in component state for as long as the form is on screen
 * and is cleared as soon as the request completes, successfully or not. The
 * session credential is never seen by this component at all: it comes back as
 * an HttpOnly cookie, so there is nothing here to store or leak.
 *
 * An account with a passkey takes two steps. The password earns a pending
 * token, and the passkey turns it into a session. The second step waits for a
 * click rather than starting on its own, because Safari refuses a WebAuthn
 * prompt that no longer has the user's gesture behind it.
 */
export function LoginForm() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [pendingToken, setPendingToken] = useState<string | null>(null);
  const [usingCode, setUsingCode] = useState(false);
  const [recoveryCode, setRecoveryCode] = useState("");
  const [supported, setSupported] = useState(false);

  // Read after mount: the server render has no window to ask.
  useEffect(() => setSupported(passkeysSupported()), []);

  function signedIn() {
    router.replace("/projects");
    router.refresh();
  }

  async function post(path: string, body: unknown): Promise<unknown> {
    const response = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify(body),
    });
    const payload = await response.json().catch(() => null);
    if (!response.ok) {
      throw new Error(
        (payload as ErrorPayload)?.error?.message ?? "Sign-in failed.",
      );
    }
    return payload;
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setPending(true);

    try {
      const response = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ email, password }),
      });

      const payload = await response.json().catch(() => null);

      if (!response.ok) {
        // The server answers every failure identically, so there is nothing
        // here to distinguish "no such account" from "wrong password".
        setError((payload as ErrorPayload)?.error?.message ?? "Sign-in failed.");
        return;
      }

      if (payload?.mfaRequired && typeof payload.pendingToken === "string") {
        setPendingToken(payload.pendingToken);
        return;
      }

      signedIn();
    } catch {
      setError("Could not reach the server.");
    } finally {
      // Cleared on every path, including the successful one.
      setPassword("");
      setPending(false);
    }
  }

  async function confirmWithPasskey() {
    if (!pendingToken) return;
    setError(null);
    setPending(true);

    try {
      const options = await post("/api/auth/passkeys/assert/begin", {
        pendingToken,
      });
      const credential = await getCredential(
        options as Parameters<typeof getCredential>[0],
      );
      await post("/api/auth/passkeys/assert/finish", {
        pendingToken,
        credential,
      });
      signedIn();
    } catch (caught) {
      setError(describePasskeyError(caught));
    } finally {
      setPending(false);
    }
  }

  async function confirmWithRecoveryCode(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!pendingToken) return;
    setError(null);
    setPending(true);

    try {
      await post("/api/auth/passkeys/assert/recovery", {
        pendingToken,
        recoveryCode,
      });
      signedIn();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Sign-in failed.");
    } finally {
      setRecoveryCode("");
      setPending(false);
    }
  }

  function startOver() {
    setPendingToken(null);
    setUsingCode(false);
    setRecoveryCode("");
    setError(null);
  }

  async function signInWithPasskey() {
    setError(null);
    setPending(true);

    try {
      const begun = (await post("/api/auth/passkeys/signin/begin", {})) as {
        handle: string;
        options: Parameters<typeof getCredential>[0];
      };
      const credential = await getCredential(begun.options);
      await post("/api/auth/passkeys/signin/finish", {
        handle: begun.handle,
        kind: "browser",
        credential,
      });
      signedIn();
    } catch (caught) {
      setError(describePasskeyError(caught));
    } finally {
      setPending(false);
    }
  }

  if (pendingToken && usingCode) {
    return (
      <form onSubmit={confirmWithRecoveryCode} className="space-y-3.5">
        <p className="prose-note text-muted-foreground">
          Enter one of the recovery codes you saved. It works once, and you can
          issue a new set from Settings once you&apos;re in.
        </p>

        <Input
          id="recovery-code"
          name="recovery-code"
          label="Recovery code"
          autoComplete="one-time-code"
          spellCheck={false}
          autoFocus
          required
          value={recoveryCode}
          onChange={setRecoveryCode}
        />

        {error && (
          <p role="alert" className="text-meta text-destructive">
            {error}
          </p>
        )}

        <Button type="submit" size="lg" className="w-full" loading={pending}>
          Continue
          {!pending && <ArrowRight />}
        </Button>

        <div className="flex items-center justify-between text-meta text-muted-foreground">
          <button
            type="button"
            className="underline underline-offset-4 hover:opacity-70"
            onClick={startOver}
          >
            Back
          </button>
          <button
            type="button"
            className="underline underline-offset-4 hover:opacity-70"
            onClick={() => {
              setUsingCode(false);
              setError(null);
            }}
          >
            Use passkey instead
          </button>
        </div>
      </form>
    );
  }

  if (pendingToken) {
    return (
      <div className="space-y-3.5">
        <p className="prose-note text-muted-foreground">
          Your password was accepted. Confirm it&apos;s you with the passkey on
          this account.
        </p>

        {error && (
          <p role="alert" className="text-meta text-destructive">
            {error}
          </p>
        )}

        <Button
          type="button"
          size="lg"
          className="w-full"
          loading={pending}
          onClick={confirmWithPasskey}
          autoFocus
        >
          {!pending && <Fingerprint />}
          Use passkey
        </Button>

        <div className="flex items-center justify-between text-meta text-muted-foreground">
          <button
            type="button"
            className="underline underline-offset-4 hover:opacity-70"
            onClick={startOver}
          >
            Back
          </button>
          <button
            type="button"
            className="underline underline-offset-4 hover:opacity-70"
            onClick={() => {
              setUsingCode(true);
              setError(null);
            }}
          >
            Use a recovery code
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3.5">
      <form onSubmit={onSubmit} className="space-y-3.5">
        <Input
          id="email"
          name="email"
          label="Email"
          type="email"
          // "webauthn" lets the browser offer saved passkeys from this field.
          autoComplete="username webauthn"
          autoFocus
          required
          value={email}
          onChange={setEmail}
        />

        <Input
          id="password"
          name="password"
          label="Password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={setPassword}
        />

        <div className="-mt-1.5 flex justify-end">
          <Link
            href="/forgot-password"
            className="text-meta text-muted-foreground underline underline-offset-4 hover:opacity-70"
          >
            Forgot password?
          </Link>
        </div>

        {error && (
          // Rendered as text, never as markup. Every value in this interface
          // reaches the DOM through React's escaping.
          <p role="alert" className="text-meta text-destructive">
            {error}
          </p>
        )}

        <Button type="submit" size="lg" className="w-full" loading={pending}>
          Continue
          {!pending && <ArrowRight />}
        </Button>
      </form>

      {supported && (
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="w-full"
          disabled={pending}
          onClick={signInWithPasskey}
        >
          <Fingerprint />
          Sign in with a passkey
        </Button>
      )}
    </div>
  );
}
