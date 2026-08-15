"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  Bot,
  Check,
  Clock,
  GitBranch,
  Minus,
  Server,
  ShieldCheck,
  Trash2,
  User,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DataList,
  DataListHeader,
  DataListRow,
  EmptyState,
} from "@/components/ui/data-list";
import { useToast } from "@/components/ui/toast";
import { RelativeTime } from "@/components/ui/relative-time";
import { apiFetch } from "@/lib/client-api";


export type Grant = {
  id: string;
  subjectType: "USER" | "MACHINE";
  subjectId: string;
  subjectName: string;
  subjectKind: string;
  scope: string;
  environmentId: string | null;
  capabilities: string[];
  canUse: boolean;
  canReveal: boolean;
  expiresAt: string | null;
  reason: string;
  temporary: boolean;
};

type Environment = { id: string; name: string; slug: string };

/**
 * The access screen.
 *
 * Its whole job is to make one distinction obvious at a glance: using a
 * credential and seeing it are different things. So the two capabilities get
 * their own columns, their own colours, and an explicit mark for "no" rather
 * than an empty cell — because a blank space reads as "not loaded yet" and a
 * dash reads as "definitely not".
 */
export function AccessMatrix({
  projectId,
  grants,
  environments,
}: {
  projectId: string;
  grants: Grant[];
  environments: Environment[];
}) {
  const byEnvironment = new Map<string, Grant[]>();
  const projectWide: Grant[] = [];

  for (const grant of grants) {
    if (!grant.environmentId) {
      projectWide.push(grant);
      continue;
    }
    const existing = byEnvironment.get(grant.environmentId) ?? [];
    existing.push(grant);
    byEnvironment.set(grant.environmentId, existing);
  }

  if (grants.length === 0) {
    return (
      <DataList>
        <EmptyState
          icon={<ShieldCheck className="size-5" />}
          title="No access granted yet"
          description="Nothing can use these secrets until something is granted USE_SECRET. Roles do not confer it — not even owner."
        />
      </DataList>
    );
  }

  return (
    <div className="space-y-3">
      {projectWide.length > 0 && (
        <GrantGroup
          projectId={projectId}
          title="All environments"
          subtitle="project-wide"
          grants={projectWide}
        />
      )}

      {environments.map((environment) => {
        const forEnvironment = byEnvironment.get(environment.id) ?? [];
        if (forEnvironment.length === 0) return null;
        return (
          <GrantGroup
            key={environment.id}
            projectId={projectId}
            title={environment.name}
            subtitle={environment.slug}
            grants={forEnvironment}
          />
        );
      })}
    </div>
  );
}

function GrantGroup({
  projectId,
  title,
  subtitle,
  grants,
}: {
  projectId: string;
  title: string;
  subtitle: string;
  grants: Grant[];
}) {
  return (
    <DataList>
      <DataListHeader className="normal-case tracking-normal">
        <span className="text-body font-medium text-foreground">{title}</span>
        <span className="font-mono text-meta normal-case text-muted-foreground">
          {subtitle}
        </span>

        <div className="ml-auto flex items-center gap-3">
          <span className="w-20 text-right">Can use</span>
          <span className="w-20 text-right">Can see</span>
          <span className="w-24 text-right">Expires</span>
          <span className="w-7" />
        </div>
      </DataListHeader>

      {grants.map((grant) => (
        <GrantRow key={grant.id} projectId={projectId} grant={grant} />
      ))}
    </DataList>
  );
}

function GrantRow({ projectId, grant }: { projectId: string; grant: Grant }) {
  const router = useRouter();
  const toast = useToast();
  const [pending, setPending] = useState(false);

  async function revoke() {
    setPending(true);
    try {
      await apiFetch(`/api/projects/${projectId}/access/${grant.id}`, {
        method: "DELETE",
      });
      toast.success(
        `Access revoked for ${grant.subjectName}. The next request is denied.`,
      );
      router.refresh();
    } catch (caught) {
      toast.error(
        caught instanceof Error
          ? caught.message
          : "Could not revoke this access.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <DataListRow>
      {/* <SubjectIcon
        subjectType={grant.subjectType}
        subjectKind={grant.subjectKind}
      /> */}

      <div className="min-w-0 flex-1">
        <div className="truncate text-body">{grant.subjectName || "Unknown"}</div>
        <div className="truncate text-meta text-muted-foreground">
          {grant.subjectKind.toLowerCase().replace("_", " ")}
          {grant.reason && <span className="italic"> · {grant.reason}</span>}
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-3">
        <div className="w-20 text-right">
          <CapabilityMark
            granted={grant.canUse}
            variant="use"
            label="USE_SECRET"
          />
        </div>
        <div className="w-20 text-right">
          <CapabilityMark
            granted={grant.canReveal}
            variant="reveal"
            label="READ_SECRET"
          />
        </div>

        <div className="w-24 text-right text-meta tabular text-muted-foreground">
          {grant.temporary ? (
            <span className="inline-flex items-center gap-1">
              <Clock className="size-3" />
              <RelativeTime value={grant.expiresAt} />
            </span>
          ) : (
            "Never"
          )}
        </div>

        <Button
          variant="ghost"
          size="icon"
          className="size-7"
          onClick={revoke}
          disabled={pending}
          aria-label={`Revoke access for ${grant.subjectName}`}
          title="Revoke — runtime access is denied on the next request"
        >
          <Trash2 />
        </Button>
      </div>
    </DataListRow>
  );
}

/**
 * A capability, marked present or absent.
 *
 * "No" is drawn explicitly rather than left blank. On a screen whose purpose is
 * to show that someone can use a credential without seeing it, the absence has
 * to be as visible as the presence.
 */
function CapabilityMark({
  granted,
  variant,
  label,
}: {
  granted: boolean;
  variant: "use" | "reveal";
  label: string;
}) {
  if (!granted) {
    return (
      <span
        className="inline-flex items-center gap-1 text-meta text-muted-foreground"
        title={`Not granted ${label}`}
      >
        <Minus className="size-3" />
        No
      </span>
    );
  }

  return (
    <Badge variant={variant} title={`Granted ${label}`}>
      <Check className="size-3" />
      Yes
    </Badge>
  );
}

export function SubjectIcon({
  subjectType,
  subjectKind,
}: {
  subjectType: string;
  subjectKind: string;
}) {
  const Icon =
    subjectType === "USER"
      ? User
      : subjectKind === "AI_AGENT"
        ? Bot
        : subjectKind === "CI"
          ? GitBranch
          : Server;

  return (
    <div className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-muted/50">
      <Icon className="size-3.5 text-muted-foreground" />
    </div>
  );
}
