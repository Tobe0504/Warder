import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ tokenId: string }> };

export async function POST(request: Request, { params }: Params) {
  const { tokenId } = await params;
  return handleRoute(request, () =>
    callCoreApi(`/tokens/${requireId(tokenId)}/revoke`, { method: "POST" }),
  );
}
