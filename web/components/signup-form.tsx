"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Check, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
  const [organizationName, setOrganizationName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);

  const passwordLongEnough = password.length >= MIN_PASSWORD_LENGTH;

  function onOrganizationNameChange(value: string) {
    setOrganizationName(value);
    if (!slugEdited) {
      setSlug(
        value
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, "-")
          .replace(/^-+|-+$/g, "")
          .slice(0, 63),
      );
    }
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setFieldErrors({});

    if (!passwordLongEnough) {
      setFieldErrors({ password: `Use at least ${MIN_PASSWORD_LENGTH} characters.` });
      return;
    }

    setPending(true);
    try {
      await apiFetch("/api/organizations", {
        method: "POST",
        body: { organizationName, slug, name, email, password },
      });

      router.replace("/projects");
      router.refresh();
    } catch (caught) {
      if (caught instanceof ApiError && caught.details) {
        setFieldErrors(caught.details);
        setError(caught.message);
      } else {
        setError(caught instanceof Error ? caught.message : "Could not create the organization.");
      }
    } finally {
      // Cleared on every path, including success.
      setPassword("");
      setPending(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3.5">
      <Field label="Organization" htmlFor="organizationName" error={fieldErrors.organizationName}>
        <Input
          id="organizationName"
          value={organizationName}
          onChange={(e) => onOrganizationNameChange(e.target.value)}
          placeholder="Acme"
          autoComplete="organization"
          autoFocus
          required
          className="h-9"
        />
      </Field>

      <Field label="Slug" htmlFor="slug" error={fieldErrors.slug}>
        <Input
          id="slug"
          value={slug}
          onChange={(e) => {
            setSlugEdited(true);
            setSlug(e.target.value);
          }}
          placeholder="acme"
          className="font-mono"
          pattern="[a-z0-9]([a-z0-9-]*[a-z0-9])?"
          required
        />
      </Field>

      <Field label="Your name" htmlFor="name" error={fieldErrors.name}>
        <Input
          id="name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoComplete="name"
          required
        />
      </Field>

      <Field label="Email" htmlFor="email" error={fieldErrors.email}>
        <Input
          id="email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoComplete="username"
          required
        />
      </Field>

      <Field label="Password" htmlFor="password" error={fieldErrors.password}>
        <Input
          id="password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          aria-describedby="password-requirement"
          required
        />
        <p
          id="password-requirement"
          className={cn(
            "flex items-center gap-1 text-meta",
            password.length === 0
              ? "text-muted-foreground"
              : passwordLongEnough
                ? "text-can-use"
                : "text-muted-foreground",
          )}
        >
          {password.length > 0 &&
            (passwordLongEnough ? <Check className="size-3" /> : <X className="size-3" />)}
          At least {MIN_PASSWORD_LENGTH} characters. Length matters more than symbols.
        </p>
      </Field>

      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}

      <Button type="submit" size="lg" className="w-full" disabled={pending}>
        {pending ? "Creating…" : "Create organization"}
        {!pending && <ArrowRight />}
      </Button>
    </form>
  );
}

function Field({
  label,
  htmlFor,
  error,
  children,
}: {
  label: string;
  htmlFor: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
