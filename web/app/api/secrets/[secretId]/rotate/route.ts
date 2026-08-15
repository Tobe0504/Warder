import { callCoreApi } from "@/lib/core-api";
import { handleRoute, optionalString, readJson, requireId, requireString } from "@/lib/route-helpers";

type Params = { params: Promise<{ secretId: string }> };

export async function POST(request: Request, { params }: Params) {
  const { secretId } = await params;
  return handleRoute(request, async () => {
    const body = await readJson(request);
    return callCoreApi(`/secrets/${requireId(secretId)}/rotate`, {
      method: "POST",
      body: {
        value: requireString(body, "value", 512 * 1024),
        expiresAt: optionalString(body, "expiresAt", 64),
      },
    });
  });
}
