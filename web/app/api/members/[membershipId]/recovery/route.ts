import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ membershipId: string }> };

/**
 * Issues a one-time recovery link for a member.
 *
 * The response carries the link once. It is stored hashed, so there is no
 * screen that can show it again.
 */
export async function POST(request: Request, { params }: Params) {
  const { membershipId } = await params;
  return handleRoute(request, () =>
    callCoreApi(`/members/${requireId(membershipId)}/recovery`, {
      method: "POST",
      body: {},
    }),
  );
}
