import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson } from "@/lib/route-helpers";

/**
 * Issues the challenge for a password that has already been accepted.
 *
 * No session exists yet: the caller holds a pending token and nothing else.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);
    const body = await readJson(request);
    return callCoreApi("/auth/passkeys/assert/begin", {
      method: "POST",
      anonymous: true,
      body,
    });
  });
}
