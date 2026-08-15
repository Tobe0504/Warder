import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson, requireString } from "@/lib/route-helpers";

/**
 * Redeems an invitation and creates the account.
 *
 * Runs before any session exists, so there is no CSRF token to check — but the
 * origin still has to be right, for the same reason sign-in checks it: without
 * it another site could drive somebody's browser through an acceptance they did
 * not intend.
 *
 * Neither the address nor the role is sent from here. The core API reads both
 * from the invitation row, so nothing the browser supplies can change who the
 * new account is or what it may do.
 *
 * No session is issued. The invitee signs in afterwards, which proves the
 * password was stored as typed and keeps this from becoming a second way to
 * mint a session.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);

    const body = await readJson(request);

    return callCoreApi("/auth/accept-invitation", {
      method: "POST",
      anonymous: true,
      body: {
        // Arrives in the body rather than the URL: the token is a credential,
        // and a credential in a path or a query string is a credential in an
        // access log.
        token: requireString(body, "token", 128),
        name: requireString(body, "name", 128),
        password: requireString(body, "password", 256),
      },
    });
  });
}
