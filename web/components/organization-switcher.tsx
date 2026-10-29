"use client";

import { useEffect, useState } from "react";
import { Building2, Check, ChevronsUpDown } from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";
import { cn } from "@/lib/utils";
import { Loader } from "@/components/motion/loader";

type Membership = {
  organizationId: string;
  name: string;
  slug: string;
  role: string;
  current: boolean;
};

/**
 * Moves between the organizations one person belongs to.
 *
 * The list is fetched when the menu opens rather than with the page. Most
 * people are in exactly one organization and would never open this, and asking
 * the core API on every dashboard render to populate a menu nobody touches is a
 * request per page view for nothing.
 *
 * Switching replaces the session cookie server-side, so the page is reloaded
 * rather than refreshed: every cached server-component payload the router holds
 * was rendered for the organization being left.
 */
export function OrganizationSwitcher({
  organization,
}: {
  organization: string;
}) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [memberships, setMemberships] = useState<Membership[] | null>(null);
  const [switching, setSwitching] = useState<string | null>(null);

  useEffect(() => {
    if (!open || memberships !== null) return;

    let cancelled = false;
    apiFetch<{ organizations: Membership[] | null }>("/api/organizations")
      .then((result) => {
        if (!cancelled) setMemberships(result.organizations ?? []);
      })
      .catch(() => {
        // An empty list renders as "only this one", which is the truth for
        // almost everybody and is not worth a toast.
        if (!cancelled) setMemberships([]);
      });

    return () => {
      cancelled = true;
    };
  }, [open, memberships]);

  async function select(membership: Membership) {
    if (membership.current) {
      setOpen(false);
      return;
    }

    setSwitching(membership.slug);
    try {
      await apiFetch("/api/organizations/switch", {
        method: "POST",
        body: { organization: membership.slug },
      });
      // A full load, not router.refresh(): the session cookie changed and every
      // cached payload belongs to the previous organization.
      window.location.href = "/projects";
    } catch (caught) {
      toast.error(
        caught instanceof Error
          ? caught.message
          : "Could not switch organization.",
      );
      setSwitching(null);
    }
  }

  const others = (memberships ?? []).filter((m) => !m.current);

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`Organization: ${organization}. Switch organization`}
          className="flex min-w-0 flex-1 items-center gap-1.5 rounded-md px-1.5 py-1 text-left outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-muted"
        >
          <span className="truncate text-meta font-semibold text-foreground">
            {organization}
          </span>
          <ChevronsUpDown
            aria-hidden="true"
            className="size-3.5 shrink-0 text-muted-foreground"
          />
        </button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="start" className="w-60">
        <DropdownMenuLabel className="text-meta font-normal text-muted-foreground">
          Organizations
        </DropdownMenuLabel>

        {memberships === null ? (
          <div className="px-2 py-1.5 text-meta text-muted-foreground">
            <Loader size={16} />
          </div>
        ) : (
          <>
            {memberships.map((membership) => (
              <DropdownMenuItem
                key={membership.organizationId}
                disabled={switching !== null}
                onSelect={(event) => {
                  event.preventDefault();
                  void select(membership);
                }}
              >
                <Building2 />
                <span className="min-w-0 flex-1 truncate">
                  {membership.name}
                </span>
                <span
                  className={cn(
                    "shrink-0 text-meta text-muted-foreground",
                    membership.current && "hidden",
                  )}
                >
                  {membership.role.toLowerCase()}
                </span>
                {membership.current && <Check className="shrink-0" />}
              </DropdownMenuItem>
            ))}

            {others.length === 0 && memberships.length > 0 && (
              <>
                <DropdownMenuSeparator />
                <div className="px-2 py-1.5 text-meta leading-snug text-muted-foreground">
                  You belong to one organization. An owner or admin of another
                  can invite this address to theirs.
                </div>
              </>
            )}
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
