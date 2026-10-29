import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute } from "@/lib/route-helpers";

/**
 * Issues a challenge for a sign-in that names no account.
 *
 * Nothing identifies the person here. The passkey carries the account handle,
 * which is what keeps this endpoint from being a way to ask whether an address
 * has an account.
 */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);
    return callCoreApi("/auth/passkeys/signin/begin", {
      method: "POST",
      anonymous: true,
      body: {},
    });
  });
}
