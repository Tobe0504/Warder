import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ membershipId: string }> };

/**
 * Removes a member.
 *
 * Their sessions stop working on the next request, and no credential is
 * rotated: because they never held one. That is the whole argument of the
 * product, exercised on the day it matters.
 */
export async function DELETE(request: Request, { params }: Params) {
  const { membershipId } = await params;
  return handleRoute(request, () =>
    callCoreApi(`/members/${requireId(membershipId)}`, { method: "DELETE" }),
  );
}
