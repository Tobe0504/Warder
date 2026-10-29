"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";
import { ApiError, apiFetch } from "@/lib/client-api";

/**
 * Setting a new password with a recovery code.
 *
 * The only self-service way back into an account with no passkey, because
 * Warder sends no email and so has no reset link to send.
 */
export function ForgotPasswordForm() {
  const router = useRouter();

  const [email, setEmail] = useState("");
  const [recoveryCode, setRecoveryCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [remaining, setRemaining] = useState<number | null>(null);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setFieldErrors({});
    setPending(true);

    try {
      const result = await apiFetch<{ remainingCodes: number }>(
        "/api/auth/password/reset",
        { method: "POST", body: { email, recoveryCode, newPassword } },
      );
      setRemaining(result.remainingCodes);
    } catch (caught) {
      if (caught instanceof ApiError && caught.details) {
        setFieldErrors(caught.details);
      }
      setError(
        caught instanceof Error
          ? caught.message
          : "That did not work. Check the code and try again.",
      );
      setPending(false);
    } finally {
      // Cleared on every path, including success.
      setNewPassword("");
    }
  }

  if (remaining !== null) {
    return (
      <div className="space-y-4 text-center">
        <p className="prose-note text-muted-foreground">
          Your password is set and every session has been signed out. You have{" "}
          <span className="font-medium text-foreground">{remaining}</span>{" "}
          {remaining === 1 ? "code" : "codes"} left.
          {remaining <= 2 && " Generate a new set from Settings once you are in."}
        </p>
        <Button className="w-full" onClick={() => router.replace("/login")}>
          Sign in
          <ArrowRight />
        </Button>
      </div>
    );
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3.5">
      <Input
        id="email"
        label="Email"
        type="email"
        value={email}
        onChange={setEmail}
        autoFocus
        required
      />

      <Input
        id="recoveryCode"
        label="Recovery code"
        value={recoveryCode}
        onChange={setRecoveryCode}
        error={fieldErrors.recoveryCode}
        placeholder="7KQ4N-2WX8F-MP3RT"
        classNames={{ input: "font-mono" }}
        autoComplete="off"
        spellCheck={false}
        required
      />

      <Input
        id="newPassword"
        label="New password"
        type="password"
        value={newPassword}
        onChange={setNewPassword}
        error={fieldErrors.newPassword}
        required
      />
      <p className="prose-note text-muted-foreground">
        At least 12 characters. Every session signs out when this changes.
      </p>

      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}

      <Button type="submit" size="lg" className="w-full" loading={pending}>
        Set a new password
      </Button>
    </form>
  );
}
