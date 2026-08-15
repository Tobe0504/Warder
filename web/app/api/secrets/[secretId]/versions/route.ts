import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ secretId: string }> };

export async function GET(request: Request, { params }: Params) {
  const { secretId } = await params;
  return handleRoute(request, () => callCoreApi(`/secrets/${requireId(secretId)}/versions`));
}
