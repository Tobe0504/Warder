import { assertSameOrigin } from "@/lib/csrf";
import { callCoreApi } from "@/lib/core-api";
import { handlePreAuthRoute, readJson } from "@/lib/route-helpers";

/** Redeems an owner-issued recovery link. */
export async function POST(request: Request) {
  return handlePreAuthRoute(async () => {
    await assertSameOrigin(request);
    const body = await readJson(request);
    return callCoreApi("/auth/password/recover", {
      method: "POST",
      anonymous: true,
      body,
    });
  });
}
