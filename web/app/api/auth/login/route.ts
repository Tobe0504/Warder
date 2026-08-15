import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson, requireString } from "@/lib/route-helpers";
import { startSession } from "@/lib/session";

type LoginResponse = {
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
    // There is no CSRF token to check yet — no session exists — but the origin
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
