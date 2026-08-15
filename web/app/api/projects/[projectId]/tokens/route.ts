import { callCoreApi } from "@/lib/core-api";
import {
  handleRoute,
  optionalString,
  readJson,
  requireId,
  requireString,
  stringList,
} from "@/lib/route-helpers";

type Params = { params: Promise<{ projectId: string }> };

export async function GET(request: Request, { params }: Params) {
  const { projectId } = await params;
  return handleRoute(request, () => callCoreApi(`/projects/${requireId(projectId)}/tokens`));
}

/**
 * Mints a runtime token.
 *
 * The response carries the full credential exactly once. It is passed straight
 * to the caller for display and is not stored here, not logged, and not
 * retrievable afterwards — the core API keeps only a verifier.
 */
export async function POST(request: Request, { params }: Params) {
  const { projectId } = await params;
  return handleRoute(request, async () => {
    const body = await readJson(request);
    return callCoreApi(`/projects/${requireId(projectId)}/tokens`, {
      method: "POST",
      body: {
        identityId: requireString(body, "identityId", 64),
        name: requireString(body, "name", 128),
        environmentId: requireString(body, "environmentId", 64),
        capabilities: stringList(body, "capabilities", 16),
        secretKeys: stringList(body, "secretKeys", 128),
        expiresAt: optionalString(body, "expiresAt", 64),
      },
    });
  });
}
