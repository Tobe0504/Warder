import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson, requireString } from "@/lib/route-helpers";

/**
 * Reports what an invitation is for, before anything is typed.
 *
 * The invite page has three shapes and no way to choose between them without
 * this: join with the account already signed in, sign in first, or create an
 * account. Guessing wrong means asking somebody to invent a password for an
 * account they already have.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);

    const body = await readJson(request);

    return callCoreApi("/auth/invitations/inspect", {
      method: "POST",
      optionalSession: true,
      body: { token: requireString(body, "token", 128) },
    });
  });
}
