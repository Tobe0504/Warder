"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import {
  Ban,
  Bot,
  GitBranch,
  MoreHorizontal,
  Server,
  Workflow,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DataList,
  DataListRow,
  EmptyState,
  RowMeta,
  StatusDot,
} from "@/components/ui/data-list";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useToast } from "@/components/ui/toast";
import { RelativeTime } from "@/components/ui/relative-time";
import { apiFetch } from "@/lib/client-api";

export type Identity = {
  id: string;
  name: string;
  actorType: "SERVICE" | "AI_AGENT" | "CI" | "WORKLOAD";
  createdAt: string;
  expiresAt: string | null;
  active: boolean;
};

export function IdentitiesTable({
  identities,
  emptyAction,
}: {
  identities: Identity[];
  emptyAction?: React.ReactNode;
}) {
  if (identities.length === 0) {
    return (
      <DataList>
        <EmptyState
          icon={<Workflow className="size-5" />}
          title="No identities yet"
          description="Create one for each thing that runs your code — the API in production, the CI job, an agent working on a branch. Separate identities mean you can revoke one without disturbing the others."
          action={emptyAction}
        />
      </DataList>
    );
  }

  return (
    <DataList>
      {identities.map((identity) => (
        <IdentityRow key={identity.id} identity={identity} />
      ))}
    </DataList>
  );
}

function IdentityRow({ identity }: { identity: Identity }) {
  const router = useRouter();
  const toast = useToast();
  const [confirming, setConfirming] = useState(false);
  const [pending, setPending] = useState(false);

  async function revoke() {
    setPending(true);
    try {
      await apiFetch(`/api/identities/${identity.id}/disable`, {
        method: "POST",
      });
      setConfirming(false);
      toast.success(
        `${identity.name} revoked. Its tokens and sessions stop working immediately.`,
      );
      router.refresh();
    } catch (caught) {
      toast.error(
        caught instanceof Error
          ? caught.message
          : "Could not revoke this identity.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      <DataListRow className={identity.active ? "" : "opacity-55"}>
        <div className="min-w-0 flex-1">
          <div className="truncate text-body">{identity.name}</div>
          <div className="mt-0.5 text-meta text-muted-foreground">
            {label(identity.actorType)}
          </div>
        </div>

        {identity.actorType === "AI_AGENT" && (
          <Badge variant="reveal" className="shrink-0">
            agent
          </Badge>
        )}

        <StatusDot
          tone={identity.active ? "ready" : "error"}
          label={identity.active ? "active" : "revoked"}
        />

        <RowMeta>
          <span title="When this identity stops being able to authenticate">
            <RelativeTime
              value={identity.expiresAt}
              prefix="Expires"
              fallback="No expiry"
            />
          </span>
          <RelativeTime
            className="hidden md:inline"
            value={identity.createdAt}
            prefix="Created"
          />

          {identity.active ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  aria-label={`Actions for ${identity.name}`}
                >
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem
                  destructive
                  onSelect={() => setConfirming(true)}
                >
                  <Ban />
                  Revoke identity
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : (
            <span className="w-7" />
          )}
        </RowMeta>
      </DataListRow>

      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Revoke <span className="font-mono">{identity.name}</span>?
            </DialogTitle>
            <DialogDescription>
              This cannot be undone. There is no way to bring an identity back —
              create a new one if you need it again.
            </DialogDescription>
          </DialogHeader>

          {/*
            Says what actually happens, in the order it happens. "Are you sure?"
            on its own asks someone to guess at the consequences of a security
            action, which is exactly when they should not have to.
          */}
          <ul className="space-y-1.5 rounded-md bg-muted px-3 py-2.5 prose-note text-muted-foreground">
            <li>
              · Every token issued to it stops authenticating on the next
              request.
            </li>
            <li>
              · Sessions already issued from those tokens are revoked now, not
              when they lapse.
            </li>
            <li>
              · Anything running with secrets it already fetched keeps them
              until it restarts.
            </li>
            <li>· No secret is changed, and nothing else loses access.</li>
          </ul>

          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setConfirming(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={revoke}
              disabled={pending}
            >
              {pending ? "Revoking…" : "Revoke identity"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export function ActorIcon({ actorType }: { actorType: Identity["actorType"] }) {
  const className = "size-3.5 text-muted-foreground";
  switch (actorType) {
    case "AI_AGENT":
      return <Bot className={className} />;
    case "CI":
      return <GitBranch className={className} />;
    case "WORKLOAD":
      return <Server className={className} />;
    default:
      return <Workflow className={className} />;
  }
}

function label(actorType: Identity["actorType"]): string {
  switch (actorType) {
    case "AI_AGENT":
      return "AI agent";
    case "CI":
      return "CI pipeline";
    case "WORKLOAD":
      return "Workload";
    default:
      return "Service";
  }
}
