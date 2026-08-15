import { callCoreApi } from "@/lib/core-api";
import { handleRoute } from "@/lib/route-helpers";
import { endSession } from "@/lib/session";

/**
 * Sign-out.
 *
 * The session is revoked on the server first, then the cookies are cleared.
 * Clearing the cookie alone would leave a working credential behind for anyone
 * who had captured it.
 */
export async function POST(request: Request) {
  return handleRoute(request, async () => {
    try {
      await callCoreApi("/auth/logout", { method: "POST" });
    } catch {
      // An already-invalid session still signs the person out locally. Failing
      // here would leave them stuck on a page they cannot use.
    }
    await endSession();
    return { ok: true };
  });
}
