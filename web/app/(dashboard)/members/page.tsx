import { Crumb, PageShell, PageTitle } from "@/components/page-shell";
import { InvitationsTable, type Invitation } from "@/components/invitations-table";
import { InviteMemberDialog } from "@/components/invite-member-dialog";
import { MembersTable, type Member } from "@/components/members-table";
import { callCoreApi } from "@/lib/core-api";
import type { SessionUser } from "@/lib/session-user";

/**
 * Organization members.
 *
 * The note at the bottom is the important part of this page. A role here looks
 * like it grants access and does not: it governs administration only. Someone
 * reading a list of owners and admins should not walk away believing those
 * people can read secrets.
 */
export default async function MembersPage() {
  const [{ members }, { invitations }, user] = await Promise.all([
    callCoreApi<{ members: Member[] | null }>("/members"),
    callCoreApi<{ invitations: Invitation[] | null }>("/members/invitations"),
    callCoreApi<SessionUser>("/auth/session"),
  ]);

  const list = members ?? [];
  const allInvitations = invitations ?? [];

  // Accepted and withdrawn invitations are history, and the member list already
  // shows the people who accepted. Only what is still outstanding earns a
  // section of its own.
  const outstanding = allInvitations.filter((invitation) => invitation.status === "PENDING");

  return (
    <PageShell breadcrumb={<Crumb>Members</Crumb>}>
      <PageTitle
        title="Members"
        description="People in this organization, and the administrative role each one holds."
        actions={<InviteMemberDialog />}
      />

      <MembersTable members={list} currentUserId={user.id} />

      {outstanding.length > 0 && (
        <section className="mt-8">
          <h2 className="mb-2 text-heading font-semibold">Pending invitations</h2>
          <p className="mb-3 prose-note text-muted-foreground">
            Each link works once and expires on its own. Withdrawing one stops it immediately,
            which is what to do if a link went to the wrong place.
          </p>
          <InvitationsTable invitations={outstanding} />
        </section>
      )}

      <div className="mt-8 rounded-lg border bg-card px-3 py-2.5">
        <p className="prose-note text-muted-foreground">
          <span className="font-medium text-foreground">
            Roles govern administration, not access.
          </span>{" "}
          No role including owner, grants <span className="font-mono">USE_SECRET</span> or{" "}
          <span className="font-mono">READ_SECRET</span>. Both always come from an explicit grant
          on a project&rsquo;s Access tab, which is recorded and can be given an expiry.
        </p>
        <p className="mt-1.5 prose-note text-muted-foreground">
          Removing a member ends their sessions on the next request. No credential needs rotating,
          because they never held one.
        </p>
        <p className="mt-1.5 prose-note text-muted-foreground">
          Invitations carry no password. The person who opens the link chooses their own, so
          whoever invited them never learns it and cannot sign in as them.
        </p>
      </div>
    </PageShell>
  );
}
