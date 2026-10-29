import { redirect } from "next/navigation";

import { AuthShell } from "@/components/auth-shell";
import { CreateFirstOrganizationForm } from "@/components/create-first-organization-form";
import { callCoreApi, NotAuthenticatedError } from "@/lib/core-api";
import { getSessionToken } from "@/lib/session";
import type { SessionUser } from "@/lib/session-user";

/**
 * Where a person lands between having an account and belonging to an
 * organization.
 *
 * Deliberately outside the dashboard route group: that shell draws a sidebar
 * full of destinations this session cannot reach, and a navigation whose every
 * item answers "you do not belong to an organization yet" is worse than no
 * navigation.
 */
export const dynamic = "force-dynamic";

export default async function WelcomePage() {
  if (!(await getSessionToken())) {
    redirect("/login");
  }

  let user: SessionUser;
  try {
    user = await callCoreApi<SessionUser>("/auth/session");
  } catch (error) {
    if (error instanceof NotAuthenticatedError) {
      redirect("/login");
    }
    throw error;
  }

  // Already belongs somewhere: there is nothing to do here.
  if (user.organizationId) {
    redirect("/projects");
  }

  return (
    <AuthShell
      title="Create an organization"
      description="Projects, secrets and access all live inside one. If you are expecting an invitation to somebody else's, open the link they sent you instead."
    >
      <CreateFirstOrganizationForm />
    </AuthShell>
  );
}
