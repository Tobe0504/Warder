import type { Metadata } from "next";

import { AcceptInvitationForm } from "@/components/accept-invitation-form";
import { AuthShell } from "@/components/auth-shell";

export const metadata: Metadata = {
  title: "Accept invitation",
  // An invite link is not something to index, and there is nothing here worth
  // indexing anyway.
  robots: { index: false, follow: false },
};

/**
 * The invitee's side of the flow.
 *
 * This page is rendered without reading the token at all — it arrives in the
 * URL fragment, which never reaches a server. Everything happens in the form.
 */
export default function AcceptInvitationPage() {
  return (
    <AuthShell
      title="Join the organization"
      description="Set a password to finish creating your account. Whoever invited you will not see it."
    >
      <AcceptInvitationForm />
    </AuthShell>
  );
}
