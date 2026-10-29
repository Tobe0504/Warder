import { callCoreApi } from "@/lib/core-api";
import { handleRoute, readJson } from "@/lib/route-helpers";

/**
 * Verifies the attestation and stores the key.
 *
 * The authenticator's response is forwarded unchanged: it is a signed
 * structure, and anything this layer reshaped would fail verification.
 */
export async function POST(request: Request) {
  return handleRoute(request, async () => {
    const body = await readJson(request);
    return callCoreApi("/auth/passkeys/register/finish", {
      method: "POST",
      body,
    });
  });
}
