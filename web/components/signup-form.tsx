"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Check, X } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";
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
      <Input
        id="organizationName"
        label="Organization"
        value={organizationName}
        onChange={onOrganizationNameChange}
        placeholder="Acme"
        autoComplete="organization"
        error={fieldErrors.organizationName}
        autoFocus
        required
      />

      <Input
        id="slug"
        label="Slug"
        value={slug}
        onChange={(v) => {
          setSlugEdited(true);
          setSlug(v);
        }}
        placeholder="acme"
        classNames={{ input: "font-mono" }}
        pattern="[a-z0-9]([a-z0-9-]*[a-z0-9])?"
        error={fieldErrors.slug}
        required
      />

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
            (passwordLongEnough ? <Check className="size-3" /> : <X className="size-3" />)}
          At least {MIN_PASSWORD_LENGTH} characters. Length matters more than symbols.
        </p>
      </div>

      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}

      <Button type="submit" size="lg" className="w-full" loading={pending}>
        Create organization
        {!pending && <ArrowRight />}
      </Button>
    </form>
  );
}
