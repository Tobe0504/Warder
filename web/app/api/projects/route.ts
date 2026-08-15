import { callCoreApi } from "@/lib/core-api";
import { handleRoute, readJson, requireString } from "@/lib/route-helpers";

export async function GET(request: Request) {
  return handleRoute(request, () => callCoreApi("/projects"));
}

export async function POST(request: Request) {
  return handleRoute(request, async () => {
    const body = await readJson(request);
    // Only these two fields are forwarded. Passing the parsed body through
    // would let a caller set fields the form does not offer.
    return callCoreApi("/projects", {
      method: "POST",
      body: {
        name: requireString(body, "name", 128),
        slug: requireString(body, "slug", 64),
      },
    });
  });
}
