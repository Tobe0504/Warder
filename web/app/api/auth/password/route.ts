import { callCoreApi } from "@/lib/core-api";
import { handleRoute, readJson } from "@/lib/route-helpers";

/** Changes the caller's own password. Every session ends, including this one. */
export async function POST(request: Request) {
  return handleRoute(request, async () => {
    const body = await readJson(request);
    return callCoreApi("/auth/password", { method: "POST", body });
  });
}
