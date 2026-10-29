import { callCoreApi } from "@/lib/core-api";
import {
  handleRoute,
  readJson,
  requireString,
} from "@/lib/route-helpers";
import { startSession } from "@/lib/session";

type CreateResponse = {
  organizationId: string;
  organization: string;
  slug: string;
  userId: string;
  sessionToken?: string;
  expiresAt?: string;
};
/**
 * Creates an organization and signs the new owner straight in.
 *
 * Two upstream calls, because the core API keeps creation and authentication
 * separate: correctly, since one is a bootstrap operation and the other is a
 * credential exchange. Joining them here means the person is not asked to type
 * their password twice within ten seconds.
 *
 * A note carried over from the core API and repeated where someone deploying
 * this will read it: this endpoint is open. Anyone who can reach the dashboard
 * can create a tenant. A real deployment must gate it, an invitation, an
 * allowlisted email domain, or an operator-only path. See
 * docs/security/limitations.md.
 */
/**
 * Founds an organization, with the signed-in person as its first owner.
 *
 * The core API replaces the caller's session, because a session is bound to
 * one organization for its whole life and this one currently names none. So
 * the cookie is rewritten here, exactly as switching organizations does.
 */
export async function POST(request: Request) {
  return handleRoute(request, async () => {
    const body = await readJson(request);

    const result = await callCoreApi<CreateResponse>("/organizations", {
      method: "POST",
      body: {
        organizationName: requireString(body, "organizationName", 128),
        slug: requireString(body, "slug", 64),
      },
    });

    if (result.sessionToken && result.expiresAt) {
      await startSession(result.sessionToken, new Date(result.expiresAt));
    }

    return {
      organizationId: result.organizationId,
      organization: result.organization,
      slug: result.slug,
    };
  });
}

export async function GET(request: Request) {
  return handleRoute(request, () => callCoreApi("/auth/organizations"));
}
