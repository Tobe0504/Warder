import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ projectId: string }> };

/**
 * Reads the audit trail.
 *
 * Query parameters are rebuilt from a fixed allowlist rather than forwarded.
 * Passing the caller's query string through would let it reach the upstream URL
 * unexamined, which is how a filter parameter turns into something else.
 */
export async function GET(request: Request, { params }: Params) {
  const { projectId } = await params;
  const incoming = new URL(request.url).searchParams;

  const forwarded = new URLSearchParams();
  for (const name of ["eventType", "outcome", "environmentId", "secretId", "limit"] as const) {
    const value = incoming.get(name);
    if (value && value.length <= 64) {
      forwarded.set(name, value);
    }
  }

  const query = forwarded.toString();
  return handleRoute(request, () =>
    callCoreApi(`/projects/${requireId(projectId)}/audit${query ? `?${query}` : ""}`),
  );
}
