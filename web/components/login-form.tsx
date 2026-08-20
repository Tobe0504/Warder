"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";

/**
 * Sign-in.
 *
 * The password lives in component state for as long as the form is on screen
 * and is cleared as soon as the request completes, successfully or not. The
 * session credential is never seen by this component at all: it comes back as
 * an HttpOnly cookie, so there is nothing here to store or leak.
 */
export function LoginForm() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

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

      if (!response.ok) {
        const payload = await response.json().catch(() => null);
        // The server answers every failure identically, so there is nothing
        // here to distinguish "no such account" from "wrong password".
        setError(payload?.error?.message ?? "Sign-in failed.");
        return;
      }

      router.replace("/projects");
      router.refresh();
    } catch {
      setError("Could not reach the server.");
    } finally {
      // Cleared on every path, including the successful one.
      setPassword("");
      setPending(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3.5">
      <Input
        id="email"
        name="email"
        label="Email"
        type="email"
        autoComplete="username"
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
  );
}
