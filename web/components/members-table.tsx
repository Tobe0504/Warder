"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Clock, UserMinus, Users } from "lucide-react";

import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataList, DataListRow, EmptyState, RowMeta, StatusDot } from "@/components/ui/data-list";
import { RelativeTime } from "@/components/ui/relative-time";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

export type Member = {
  membershipId: string;
  userId: string;
  email: string;
  name: string;
  role: string;
  createdAt: string;
  expiresAt: string | null;
  revokedAt: string | null;
  active: boolean;
};

export function MembersTable({ members, currentUserId }: { members: Member[]; currentUserId: string }) {
  if (members.length === 0) {
    return (
      <DataList>
        <EmptyState icon={<Users className="size-5" />} title="No members" />
      </DataList>
    );
  }

  return (
    <DataList>
      {members.map((member) => (
        <MemberRow key={member.membershipId} member={member} currentUserId={currentUserId} />
      ))}
    </DataList>
  );
}

function MemberRow({ member, currentUserId }: { member: Member; currentUserId: string }) {
  const router = useRouter();
  const toast = useToast();
  const [pending, setPending] = useState(false);

  // Removing yourself would lock you out of the organization you administer,
  // and if you are the only owner it would leave nobody able to manage it.
  const isSelf = member.userId === currentUserId;

  async function remove() {
    setPending(true);
    try {
      await apiFetch(`/api/members/${member.membershipId}`, { method: "DELETE" });
      toast.success(`${member.name} has been removed. Their sessions stop on the next request.`);
      router.refresh();
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "Could not remove this member.");
    } finally {
      setPending(false);
    }
  }

  return (
    <DataListRow className={member.active ? "" : "opacity-55"}>
      <Avatar name={member.name} />

      <div className="min-w-0 flex-1">
        <div className="truncate text-body">{member.name}</div>
        <div className="truncate text-meta text-muted-foreground">{member.email}</div>
      </div>

      <Badge variant="outline">{member.role.toLowerCase()}</Badge>

      <StatusDot
        tone={member.active ? "ready" : "error"}
        label={member.active ? "active" : "ended"}
      />

      <RowMeta>
        {member.expiresAt ? (
          <span className="inline-flex items-center gap-1" title="This membership ends by itself">
            <Clock className="size-3" />
            <RelativeTime value={member.expiresAt} />
          </span>
        ) : (
          <span>No expiry</span>
        )}
        <RelativeTime className="hidden md:inline" value={member.createdAt} prefix="Joined" />

        {member.active && !isSelf && (
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={remove}
            disabled={pending}
            aria-label={`Remove ${member.name}`}
            title="Remove: their sessions end on the next request, and no credential rotates"
          >
            <UserMinus />
          </Button>
        )}
      </RowMeta>
    </DataListRow>
  );
}
