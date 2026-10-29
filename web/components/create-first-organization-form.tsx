"use client";

import { useState } from "react";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";
import { ApiError, apiFetch } from "@/lib/client-api";

/**
 * Founding an organization, for somebody who has an account and belongs to
 * nothing.
 *
 * The slug follows the name until it is edited by hand, because it is the name
 * the CLI uses and nobody wants to invent two names for one thing.
 */
export function CreateFirstOrganizationForm() {
  const [organizationName, setOrganizationName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);

  function onNameChange(value: string) {
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
    setPending(true);

    try {
      await apiFetch("/api/organizations", {
        method: "POST",
        body: { organizationName, slug },
      });

      // A full load, not a router push: the session cookie was replaced, and
      // every cached payload the router holds was rendered for a person who
      // belonged to nothing.
      window.location.href = "/projects";
    } catch (caught) {
      if (caught instanceof ApiError && caught.details) {
        setFieldErrors(caught.details);
        setError(caught.message);
      } else {
        setError(
          caught instanceof Error
            ? caught.message
            : "Could not create the organization.",
        );
      }
      setPending(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3.5">
      <Input
        id="organizationName"
        label="Organization name"
        value={organizationName}
        onChange={onNameChange}
        error={fieldErrors.organizationName}
        placeholder="Acme"
        autoFocus
        required
      />

      <Input
        id="slug"
        label="Slug"
        value={slug}
        onChange={(next) => {
          setSlugEdited(true);
          setSlug(next);
        }}
        error={fieldErrors.slug}
        placeholder="acme"
        required
      />

      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}

      <Button type="submit" size="lg" className="w-full" loading={pending}>
        Create organization
        <ArrowRight />
      </Button>

      <p className="prose-note text-muted-foreground">
        You will be its first owner. That lets you administer projects and
        manage access, and does not by itself let you read any secret value.
      </p>
    </form>
  );
}
