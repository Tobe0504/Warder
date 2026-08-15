import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ identityId: string }> };

/**
 * Revokes a machine identity.
 *
 * A POST rather than a DELETE: the identity row survives, so the audit trail
 * keeps resolving its name long after it has stopped being able to authenticate.
 */
export async function POST(request: Request, { params }: Params) {
  const { identityId } = await params;
  return handleRoute(request, () =>
    callCoreApi(`/identities/${requireId(identityId)}/disable`, { method: "POST" }),
  );
}
