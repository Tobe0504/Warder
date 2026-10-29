import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson } from "@/lib/route-helpers";

/**
 * Spends a recovery code to set a new password.
 *
 * Runs with no session, because the whole point is that the caller cannot sign
 * in. The origin check still applies: another site must not be able to drive
 * somebody's browser through a reset.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);
    const body = await readJson(request);
    return callCoreApi("/auth/password/reset", {
      method: "POST",
      anonymous: true,
      body,
    });
  });
}
