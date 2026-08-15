"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Ban, KeySquare } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DataList,
  DataListRow,
  EmptyState,
  MonoTag,
  RowMeta,
} from "@/components/ui/data-list";
import { useToast } from "@/components/ui/toast";
import { RelativeTime } from "@/components/ui/relative-time";
import { apiFetch } from "@/lib/client-api";

export type RuntimeToken = {
  id: string;
  name: string;
  display: string;
  identityName: string;
  actorType: string;
  capabilities: string[];
  secretKeys: string[];
  expiresAt: string | null;
  lastUsedAt: string | null;
  active: boolean;
};

export function TokensTable({ tokens }: { tokens: RuntimeToken[] }) {
  if (tokens.length === 0) {
    return (
      <DataList>
        <EmptyState
          icon={<KeySquare className="size-5" />}
          title="No runtime tokens"
          description="A token lets an application, CI job, or agent authenticate and receive the secrets its identity has been granted."
        />
      </DataList>
    );
  }

  return (
    <DataList>
      {tokens.map((token) => (
        <TokenRow key={token.id} token={token} />
      ))}
    </DataList>
  );
}

function TokenRow({ token }: { token: RuntimeToken }) {
  const router = useRouter();
  const toast = useToast();
  const [pending, setPending] = useState(false);

  async function revoke() {
    setPending(true);
    try {
      await apiFetch(`/api/tokens/${token.id}/revoke`, { method: "POST" });
      toast.success(
        `${token.name} revoked, along with any session already issued from it.`,
      );
      router.refresh();
    } catch (caught) {
      toast.error(
        caught instanceof Error
          ? caught.message
          : "Could not revoke this token.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <DataListRow
      className={
        token.active
          ? "gap-y-2 max-md:flex-wrap"
          : "gap-y-2 opacity-55 max-md:flex-wrap"
      }
    >
      <div className="min-w-0 basis-40">
        <div className="truncate text-body">{token.name}</div>
        {/*
          The display form: enough to recognize the credential in a list, never
          enough to use it. Only a verifier is stored, so the real value cannot
          be shown again even if someone asked.
        */}
        <MonoTag className="mt-0.5">{token.display}</MonoTag>
      </div>

      <div className="min-w-0 flex-1 basis-32">
        <div className="truncate text-meta text-muted-foreground">
          {token.identityName}
        </div>
        <div className="mt-0.5 flex flex-wrap gap-1">
          {token.capabilities.map((capability) => (
            <Badge
              key={capability}
              variant={capability === "READ_SECRET" ? "reveal" : "use"}
              className="font-mono"
            >
              {capability}
            </Badge>
          ))}
          {token.secretKeys.length > 0 && (
            <Badge variant="outline" title={token.secretKeys.join(", ")}>
              {token.secretKeys.length} key
              {token.secretKeys.length === 1 ? "" : "s"}
            </Badge>
          )}
        </div>
      </div>

      <RowMeta className="max-md:basis-full max-md:justify-end">
        <span title="Last time this token authenticated">
          <RelativeTime
            value={token.lastUsedAt}
            prefix="Used"
            fallback="Never used"
          />
        </span>
        <span className="hidden md:inline">
          <RelativeTime
            value={token.expiresAt}
            prefix="Expires"
            fallback="No expiry"
          />
        </span>

        {token.active ? (
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={revoke}
            disabled={pending}
            aria-label={`Revoke ${token.name}`}
            title="Revoke: this token and any session derived from it stop working immediately"
          >
            <Ban />
          </Button>
        ) : (
          <Badge variant="destructive">Revoked</Badge>
        )}
      </RowMeta>
    </DataListRow>
  );
}
