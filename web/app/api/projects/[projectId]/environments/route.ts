import { callCoreApi } from "@/lib/core-api";
import { handleRoute, readJson, requireId, requireString } from "@/lib/route-helpers";

type Params = { params: Promise<{ projectId: string }> };

export async function GET(request: Request, { params }: Params) {
  const { projectId } = await params;
  return handleRoute(request, () => callCoreApi(`/projects/${requireId(projectId)}/environments`));
}

export async function POST(request: Request, { params }: Params) {
  const { projectId } = await params;
  return handleRoute(request, async () => {
    const body = await readJson(request);
    return callCoreApi(`/projects/${requireId(projectId)}/environments`, {
      method: "POST",
      body: {
        name: requireString(body, "name", 128),
        slug: requireString(body, "slug", 64),
      },
    });
  });
}
