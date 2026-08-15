import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson, requireString } from "@/lib/route-helpers";
import { startSession } from "@/lib/session";

type CreateResponse = { organizationId: string; slug: string; userId: string };
type LoginResponse = {
  sessionToken: string;
  expiresAt: string;
  user: { email: string; name: string; organization: string; role: string };
};

/**
 * Creates an organization and signs the new owner straight in.
 *
 * Two upstream calls, because the core API keeps creation and authentication
 * separate — correctly, since one is a bootstrap operation and the other is a
 * credential exchange. Joining them here means the person is not asked to type
 * their password twice within ten seconds.
 *
 * A note carried over from the core API and repeated where someone deploying
 * this will read it: this endpoint is open. Anyone who can reach the dashboard
 * can create a tenant. A real deployment must gate it — an invitation, an
 * allowlisted email domain, or an operator-only path. See
 * docs/security/limitations.md.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    // Same reasoning as sign-in: no session exists yet, so there is no token to
    // check, but a forged cross-site request must not be able to create a
    // tenant and sign the victim's browser into it.
    await assertSameOrigin(request);

    const body = await readJson(request);

    const organizationName = requireString(body, "organizationName", 128);
    const slug = requireString(body, "slug", 64);
    const name = requireString(body, "name", 128);
    const email = requireString(body, "email", 320);
    const password = requireString(body, "password", 256);

    await callCoreApi<CreateResponse>("/organizations", {
      method: "POST",
      anonymous: true,
      body: { organizationName, slug, name, email, password },
    });

    const session = await callCoreApi<LoginResponse>("/auth/login", {
      method: "POST",
      anonymous: true,
      body: { email, password, kind: "browser" },
    });

    const csrfToken = await startSession(session.sessionToken, new Date(session.expiresAt));

    // The session credential is in an HttpOnly cookie and is deliberately not
    // part of this body.
    return {
      user: {
        email: session.user.email,
        name: session.user.name,
        organization: session.user.organization,
        role: session.user.role,
      },
      csrfToken,
    };
  });
}
