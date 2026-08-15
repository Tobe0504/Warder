import { callCoreApi } from "@/lib/core-api";
import { handleRoute } from "@/lib/route-helpers";

/**
 * The organization-wide audit trail.
 *
 * Query parameters are rebuilt from a fixed allowlist rather than forwarded, so
 * a caller's query string never reaches the upstream URL unexamined.
 */
export async function GET(request: Request) {
  const incoming = new URL(request.url).searchParams;

  const forwarded = new URLSearchParams();
  for (const name of ["eventType", "outcome", "limit"] as const) {
    const value = incoming.get(name);
    if (value && value.length <= 64) {
      forwarded.set(name, value);
    }
  }

  const query = forwarded.toString();
  return handleRoute(request, () => callCoreApi(`/audit${query ? `?${query}` : ""}`));
}
