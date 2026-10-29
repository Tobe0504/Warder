import { callCoreApi } from "@/lib/core-api";
import { handleRoute } from "@/lib/route-helpers";

export async function GET(request: Request) {
  return handleRoute(request, () => callCoreApi("/auth/passkeys"));
}
