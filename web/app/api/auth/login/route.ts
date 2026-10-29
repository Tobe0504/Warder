import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson, requireString } from "@/lib/route-helpers";
import { startSession } from "@/lib/session";

type LoginResponse = {
  // Set when the account has a passkey. The password was right, but it buys
  // only a pending token: no session exists until the passkey answers.
  mfaRequired?: boolean;
  pendingToken?: string;
  sessionToken: string;
  expiresAt: string;
  user: {
    id: string;
    email: string;
    name: string;
    organizationId: string;
    organization: string;
    role: string;
  };
};

/**
 * Sign-in.
 *
 * The session credential returned by the core API is placed straight into an
 * HttpOnly cookie and is never included in the response body. Client code never
 * sees it, so there is nothing for a scripting flaw to read and nothing to end
 * up in application state, local storage, or a client-side error report.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    // There is no CSRF token to check yet: no session exists, but the origin
    // still has to be right. Without this, another site can forge a sign-in
    // with the attacker's own credentials and leave the victim working inside
    // the attacker's organization.
    await assertSameOrigin(request);

    const body = await readJson(request);
    const email = requireString(body, "email", 320);
    const password = requireString(body, "password", 256);

    const result = await callCoreApi<LoginResponse>("/auth/login", {
      method: "POST",
      anonymous: true,
      body: { email, password, kind: "browser" },
    });

    // The pending token goes to the page because the page is what runs the
    // passkey ceremony. It is not a session: the only thing it can do is ask
    // for a challenge and present the answer, and it expires in minutes.
    if (result.mfaRequired) {
      return {
        mfaRequired: true,
        pendingToken: result.pendingToken,
        expiresAt: result.expiresAt,
      };
    }

    const csrfToken = await startSession(result.sessionToken, new Date(result.expiresAt));

    // Only what the interface needs to render the shell. Note the absence of
    // the session token.
    return {
      user: {
        email: result.user.email,
        name: result.user.name,
        organization: result.user.organization,
        role: result.user.role,
      },
      csrfToken,
    };
  });
}
