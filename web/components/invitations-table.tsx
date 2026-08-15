"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Ban, MailCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataList, DataListRow, EmptyState, MonoTag, RowMeta } from "@/components/ui/data-list";
import { RelativeTime } from "@/components/ui/relative-time";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

export type Invitation = {
  id: string;
  email: string;
  name: string;
  role: string;
  status: "PENDING" | "ACCEPTED" | "REVOKED" | "EXPIRED";
  display: string;
  createdAt: string;
  expiresAt: string;
};

export function InvitationsTable({ invitations }: { invitations: Invitation[] }) {
  if (invitations.length === 0) {
    return (
      <DataList>
        <EmptyState
          icon={<MailCheck className="size-5" />}
          title="No invitations"
          description="An invitation is a single-use link. The person who opens it sets their own password, so nobody has to send one."
        />
      </DataList>
    );
  }

  return (
    <DataList>
      {invitations.map((invitation) => (
        <InvitationRow key={invitation.id} invitation={invitation} />
      ))}
    </DataList>
  );
}

function InvitationRow({ invitation }: { invitation: Invitation }) {
  const router = useRouter();
  const toast = useToast();
  const [pending, setPending] = useState(false);

  const open = invitation.status === "PENDING";

  async function revoke() {
    setPending(true);
    try {
      await apiFetch(`/api/members/invitations/${invitation.id}`, { method: "DELETE" });
      toast.success(`The invitation for ${invitation.email} no longer works.`);
      router.refresh();
    } catch (caught) {
      toast.error(
        caught instanceof Error ? caught.message : "Could not withdraw this invitation.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <DataListRow className={open ? "gap-y-2 max-md:flex-wrap" : "gap-y-2 opacity-55 max-md:flex-wrap"}>
      <div className="min-w-0 flex-1 basis-48">
        <div className="truncate text-body">{invitation.email}</div>
        {/*
          The public handle. Enough to match a row against an invitation
          somebody is asking about, never enough to redeem it.
        */}
        <MonoTag className="mt-0.5">{invitation.display}</MonoTag>
      </div>

      <Badge variant="outline">{invitation.role.toLowerCase()}</Badge>

      <Badge variant={open ? "use" : invitation.status === "ACCEPTED" ? "outline" : "destructive"}>
        {invitation.status.toLowerCase()}
      </Badge>

      <RowMeta className="max-md:basis-full max-md:justify-end">
        <RelativeTime
          value={invitation.expiresAt}
          prefix={open ? "Expires" : "Expired"}
          className="hidden md:inline"
        />
        <RelativeTime value={invitation.createdAt} prefix="Invited" />

        {open && (
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={revoke}
            disabled={pending}
            aria-label={`Withdraw the invitation for ${invitation.email}`}
            title="Withdraw: the link stops working immediately"
          >
            <Ban />
          </Button>
        )}
      </RowMeta>
    </DataListRow>
  );
}
