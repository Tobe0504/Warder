import { callCoreApi } from "@/lib/core-api";
import { handleRoute } from "@/lib/route-helpers";

/** Issues the registration challenge. */
export async function POST(request: Request) {
  return handleRoute(request, () =>
    callCoreApi("/auth/passkeys/register/begin", { method: "POST", body: {} }),
  );
}
