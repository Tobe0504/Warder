import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ passkeyId: string }> };

export async function DELETE(request: Request, { params }: Params) {
  const { passkeyId } = await params;
  return handleRoute(request, () =>
    callCoreApi(`/auth/passkeys/${requireId(passkeyId)}`, { method: "DELETE" }),
  );
}
