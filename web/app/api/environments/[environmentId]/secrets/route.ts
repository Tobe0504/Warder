import { callCoreApi } from "@/lib/core-api";
import { handleRoute, optionalString, readJson, requireId, requireString } from "@/lib/route-helpers";

type Params = { params: Promise<{ environmentId: string }> };

export async function GET(request: Request, { params }: Params) {
  const { environmentId } = await params;
  return handleRoute(request, () =>
    callCoreApi(`/environments/${requireId(environmentId)}/secrets`),
  );
}

/**
 * Creates a secret.
 *
 * This is one of only two routes in the BFF where a plaintext value passes
 * through, the other being rotation. The value is read from the body, forwarded
 * once, and never logged, never stored in a variable that outlives the request,
 * and never echoed back in the response.
 */
export async function POST(request: Request, { params }: Params) {
  const { environmentId } = await params;
  return handleRoute(request, async () => {
    const body = await readJson(request);
    return callCoreApi(`/environments/${requireId(environmentId)}/secrets`, {
      method: "POST",
      body: {
        key: requireString(body, "key", 128),
        value: requireString(body, "value", 512 * 1024),
        description: optionalString(body, "description"),
        expiresAt: optionalString(body, "expiresAt", 64),
      },
    });
  });
}
