import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ secretId: string }> };

/**
 * Reveals a secret value to an authorized human.
 *
 * This is the only BFF route that returns plaintext to a browser, and it exists
 * only because an explicit reveal is sometimes genuinely necessary. Everything
 * about it is deliberate:
 *
 *   - It is a POST, not a GET, so the action cannot be triggered by a link, an
 *     image tag, or a prefetch, and cannot land in browser history.
 *   - Authorization is the core API's decision, requiring READ_SECRET, which no
 *     role confers.
 *   - The core API records both the request and the disclosure before the value
 *     is returned.
 *   - The response is marked no-store, and the client is expected to hold the
 *     value in a local variable for as long as it is shown and no longer.
 *
 * The value passes through this function and is not logged or retained here.
 */
export async function POST(request: Request, { params }: Params) {
  const { secretId } = await params;
  return handleRoute(request, () =>
    callCoreApi<{ value: string }>(`/secrets/${requireId(secretId)}/reveal`, { method: "POST" }),
  );
}
