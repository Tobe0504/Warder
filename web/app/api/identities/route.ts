import { callCoreApi, CoreApiError } from "@/lib/core-api";
import { handleRoute, optionalString, readJson, requireString } from "@/lib/route-helpers";

const ACTOR_TYPES = new Set(["SERVICE", "AI_AGENT", "CI", "WORKLOAD"]);

export async function GET(request: Request) {
  return handleRoute(request, () => callCoreApi("/identities"));
}

export async function POST(request: Request) {
  return handleRoute(request, async () => {
    const body = await readJson(request);

    const actorType = requireString(body, "actorType", 32);
    if (!ACTOR_TYPES.has(actorType)) {
      throw new CoreApiError(400, "invalid_request", "Choose a valid identity type.");
    }

    return callCoreApi("/identities", {
      method: "POST",
      body: {
        name: requireString(body, "name", 128),
        actorType,
        expiresAt: optionalString(body, "expiresAt", 64),
      },
    });
  });
}
