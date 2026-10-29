import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import {
  handlePreAuthRoute,
  readJson,
  requireString,
} from "@/lib/route-helpers";
import { startSession } from "@/lib/session";

type SignupResponse = {
  sessionToken: string;
  expiresAt: string;
  user: { id: string; email: string; name: string };
  recoveryCodes: string[];
};

/**
 * Creates an account. Not an organization.
 *
 * The two used to be one request, which made an account something only a first
 * owner could have. A person exists first and belongs afterwards, so this
 * returns a signed-in session with no organization, and the next screen is
 * where they create one or open an invitation.
 *
 * Runs before any session exists, so there is no CSRF token to check, but the
 * origin still has to be right: without it another site could drive somebody's
 * browser through a signup they did not intend.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);

    const body = await readJson(request);

    const session = await callCoreApi<SignupResponse>("/auth/signup", {
      method: "POST",
      anonymous: true,
      body: {
        name: requireString(body, "name", 128),
        email: requireString(body, "email", 320),
        password: requireString(body, "password", 256),
      },
    });

    const csrfToken = await startSession(
      session.sessionToken,
      new Date(session.expiresAt),
    );

    // The session credential is in an HttpOnly cookie and is deliberately not
    // part of this body. The recovery codes are, and this is the only response
    // that will ever carry them: they are stored hashed, so a screen that
    // could re-read them does not exist.
    return {
      user: session.user,
      csrfToken,
      recoveryCodes: session.recoveryCodes ?? [],
    };
  });
}
