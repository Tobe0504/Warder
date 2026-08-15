import { callCoreApi, CoreApiError } from "@/lib/core-api";
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
  return handleRoute(request, () => callCoreApi(`/projects/${requireId(projectId)}/access`));
}

/**
 * Grants access.
 *
 * Each field is read and validated individually. `allEnvironments` in
 * particular is only ever true when the client explicitly sent `true`, so a
 * missing or malformed field produces a narrow grant rather than a wildcard:
 * the direction a mistake should fail in.
 */
export async function POST(request: Request, { params }: Params) {
  const { projectId } = await params;
  return handleRoute(request, async () => {
    const body = await readJson(request);

    const subjectType = requireString(body, "subjectType", 16);
    if (subjectType !== "USER" && subjectType !== "MACHINE") {
      throw new CoreApiError(400, "invalid_request", "Choose a person or a machine identity.");
    }

    const allEnvironments = body.allEnvironments === true;

    return callCoreApi(`/projects/${requireId(projectId)}/access`, {
      method: "POST",
      body: {
        subjectType,
        subjectId: requireString(body, "subjectId", 64),
        environmentId: allEnvironments ? "" : requireString(body, "environmentId", 64),
        secretId: optionalString(body, "secretId", 64),
        allEnvironments,
        capabilities: stringList(body, "capabilities", 16),
        expiresAt: optionalString(body, "expiresAt", 64),
        reason: optionalString(body, "reason"),
      },
    });
  });
}
