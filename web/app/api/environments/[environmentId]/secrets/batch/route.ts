import { callCoreApi, CoreApiError } from "@/lib/core-api";
import { handleRoute, optionalString, readJson, requireId, requireString } from "@/lib/route-helpers";

type Params = { params: Promise<{ environmentId: string }> };

/** Matches the core API's own cap, so an oversized paste fails here first. */
const MAX_SECRETS = 100;

/**
 * Stores several secrets in one call.
 *
 * The whole batch is one transaction on the core API. This route exists so the
 * browser sends one request rather than twenty: twenty would be twenty chances
 * to be interrupted, and would trip the rate limiter partway through a paste.
 *
 * Values pass through untouched and unlogged. Nothing here inspects them, and
 * the response carries back only key names and versions.
 */
export async function POST(request: Request, { params }: Params) {
  const { environmentId } = await params;

  return handleRoute(request, async () => {
    const body = await readJson(request);

    const entries = body.secrets;
    if (!Array.isArray(entries) || entries.length === 0) {
      throw new CoreApiError(400, "invalid_request", "Add at least one secret.");
    }
    if (entries.length > MAX_SECRETS) {
      throw new CoreApiError(
        400,
        "invalid_request",
        `Import at most ${MAX_SECRETS} secrets at a time.`,
      );
    }

    const secrets = entries.map((entry) => {
      if (typeof entry !== "object" || entry === null) {
        throw new CoreApiError(400, "invalid_request", "Each secret needs a key and a value.");
      }
      const row = entry as Record<string, unknown>;
      return {
        key: requireString(row, "key", 128),
        value: requireString(row, "value", 65536),
        description: optionalString(row, "description", 512),
      };
    });

    return callCoreApi(`/environments/${requireId(environmentId)}/secrets/batch`, {
      method: "POST",
      body: {
        secrets,
        expiresAt: optionalString(body, "expiresAt", 64),
      },
    });
  });
}
