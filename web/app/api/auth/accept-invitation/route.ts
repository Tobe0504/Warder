import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import {
  handlePreAuthRoute,
  optionalString,
  readJson,
  requireString,
} from "@/lib/route-helpers";

/**
 * Redeems an invitation.
 *
 * Two callers, one route. Someone without an account sends a name and a
 * password and gets one. Someone who already has an account sends neither: the
 * session says who they are, and the invitation adds an organization to the
 * account they have. A password is a property of a person, not of a
 * membership, so there is never a second one to choose.
 *
 * May run before any session exists, so there is no CSRF token to check, but
 * the origin still has to be right, for the same reason sign-in checks it:
 * without it another site could drive somebody's browser through an acceptance
 * they did not intend.
 *
 * Neither the address nor the role is sent from here. The core API reads both
 * from the invitation row, so nothing the browser supplies can change who the
 * account is or what it may do.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);

    const body = await readJson(request);

    return callCoreApi("/auth/accept-invitation", {
      method: "POST",
      optionalSession: true,
      body: {
        // In the body rather than the URL: the token is a credential, and a
        // credential in a path or a query string is a credential in an access
        // log.
        token: requireString(body, "token", 128),
        name: optionalString(body, "name", 128),
        password: optionalString(body, "password", 256),
      },
    });
  });
}
