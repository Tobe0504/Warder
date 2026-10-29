import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson } from "@/lib/route-helpers";
import { startSession } from "@/lib/session";

type SignInResponse = {
  sessionToken: string;
  expiresAt: string;
  user: {
    id: string;
    email: string;
    name: string;
    organizationId?: string;
    organization?: string;
    role?: string;
  };
};

/**
 * Completes a passwordless sign-in. The passkey is the only factor.
 *
 * The session token never reaches the page: it goes straight into the httpOnly
 * cookie, exactly as signing in with a password does. What comes back is who
 * the person is and the CSRF token their next request needs.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);
    const body = await readJson(request);

    const result = await callCoreApi<SignInResponse>("/auth/passkeys/signin/finish", {
      method: "POST",
      anonymous: true,
      body,
    });

    const csrfToken = await startSession(
      result.sessionToken,
      new Date(result.expiresAt),
    );

    return { user: result.user, csrfToken };
  });
}
