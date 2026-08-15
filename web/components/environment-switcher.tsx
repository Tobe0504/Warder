"use client";

import Link from "next/link";

import { cn } from "@/lib/utils";

type Environment = { id: string; name: string; slug: string };

/**
 * Environment selection.
 *
 * Labels use the environment's display name — "Development", not the
 * lowercase mono `development` slug. The slug is an identifier the CLI takes;
 * it is not what this control is for, and setting proper nouns in lowercase
 * monospace made a simple choice look like configuration.
 *
 * Nothing here treats "production" as a special word — the platform does not
 * either. What distinguishes environments is which grants and tokens name
 * them, so a custom environment gets exactly the same treatment as a built-in
 * one, including in this control.
 */
export function EnvironmentSwitcher({
  projectId,
  environments,
  currentId,
}: {
  projectId: string;
  environments: Environment[];
  currentId: string;
}) {
  return (
    <div
      className="inline-flex items-center gap-0.5 rounded-lg border bg-card p-1"
      role="tablist"
      aria-label="Environments"
    >
      {environments.map((environment) => {
        const active = environment.id === currentId;
        return (
          <Link
            key={environment.id}
            href={`/projects/${projectId}/environments/${environment.id}`}
            role="tab"
            aria-selected={active}
            className={cn(
              "rounded-md px-3 py-1 text-meta transition-colors font-medium",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              active
                ? "bg-muted font-medium text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {environment.name}
          </Link>
        );
      })}
    </div>
  );
}
