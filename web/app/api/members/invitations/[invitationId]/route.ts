import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ invitationId: string }> };

/** Withdraws a pending invitation, so its link stops working. */
export async function DELETE(request: Request, { params }: Params) {
  const { invitationId } = await params;
  return handleRoute(request, () =>
    callCoreApi(`/members/invitations/${requireId(invitationId)}`, { method: "DELETE" }),
  );
}
