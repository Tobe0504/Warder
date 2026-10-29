"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Check, X } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Checkbox } from "@/components/motion/checkbox";
import { Input } from "@/components/motion/input";
import { RecoveryCodesPanel } from "@/components/recovery-codes-panel";
import { ApiError, apiFetch } from "@/lib/client-api";
import { cn } from "@/lib/utils";

const MIN_PASSWORD_LENGTH = 12;

/**
 * Creates an organization and its first owner.
 *
 * The password rule is length only, matching what the server enforces and what
 * current NIST guidance recommends: composition rules mostly produce
 * predictable substitutions, while length is what actually costs an attacker.
 * The requirement is shown as it is met rather than as an error after the fact.
 */
export function SignupForm() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);

  // Set once the account exists. Their presence is what switches this form
  // into the screen that shows them.
  const [codes, setCodes] = useState<string[] | null>(null);
  const [acknowledged, setAcknowledged] = useState(false);

  const passwordLongEnough = password.length >= MIN_PASSWORD_LENGTH;

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setFieldErrors({});

    if (!passwordLongEnough) {
      setFieldErrors({
        password: `Use at least ${MIN_PASSWORD_LENGTH} characters.`,
      });
      return;
    }

    setPending(true);
    try {
      const created = await apiFetch<{ recoveryCodes?: string[] }>(
        "/api/auth/signup",
        { method: "POST", body: { name, email, password } },
      );

      // Held in this component and nowhere else.
      //
      // Not a separate route, which would mean carrying them in storage or a
      // URL: both put a credential somewhere it outlives the moment, and one
      // of them puts it in browser history. Staying on this screen is what
      // lets the codes exist only in memory.
      setCodes(created.recoveryCodes ?? []);
    } catch (caught) {
      if (caught instanceof ApiError && caught.details) {
        setFieldErrors(caught.details);
        setError(caught.message);
      } else {
        setError(
          caught instanceof Error
            ? caught.message
            : "Could not create your account.",
        );
      }
    } finally {
      // Cleared on every path, including success.
      setPassword("");
      setPending(false);
    }
  }

  // The account exists. Everything left to do is make sure these are saved.
  if (codes) {
    return (
      <div className="space-y-4">
        <div className="rounded-md border border-can-reveal/30 bg-can-reveal-surface/40 p-3">
          <p className="text-meta font-medium text-foreground">
            Save these recovery codes
          </p>
          <p className="mt-1 text-meta leading-relaxed text-muted-foreground">
            Warder sends no email, so these are how you get back in if you
            forget your password. Each one works once, and they are stored
            hashed: this is the only time they can be shown.
          </p>
        </div>

        <RecoveryCodesPanel codes={codes} />

        <Checkbox
          checked={acknowledged}
          onCheckedChange={setAcknowledged}
          label="I have saved these somewhere I can reach without this account"
        />

        <Button
          className="w-full"
          disabled={!acknowledged}
          onClick={() => {
            router.replace("/welcome");
            router.refresh();
          }}
        >
          Continue
          <ArrowRight />
        </Button>
      </div>
    );
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3.5">
      <Input
        id="name"
        label="Your name"
        value={name}
        onChange={setName}
        autoComplete="name"
        error={fieldErrors.name}
        required
      />

      <Input
        id="email"
        label="Email"
        type="email"
        value={email}
        onChange={setEmail}
        autoComplete="username"
        error={fieldErrors.email}
        required
      />

      <div className="space-y-1.5">
        <Input
          id="password"
          label="Password"
          type="password"
          value={password}
          onChange={setPassword}
          autoComplete="new-password"
          aria-describedby="password-requirement"
          error={fieldErrors.password}
          required
        />
        <p
          id="password-requirement"
          className={cn(
            "flex items-center gap-1 px-1 text-meta",
            password.length === 0
              ? "text-muted-foreground"
              : passwordLongEnough
                ? "text-can-use"
                : "text-muted-foreground",
          )}
        >
          {password.length > 0 &&
            (passwordLongEnough ? (
              <Check className="size-3" />
            ) : (
              <X className="size-3" />
            ))}
          At least {MIN_PASSWORD_LENGTH} characters.
        </p>
      </div>

      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}

      <Button type="submit" size="lg" className="w-full" loading={pending}>
        Create account
        {!pending && <ArrowRight />}
      </Button>
    </form>
  );
}
