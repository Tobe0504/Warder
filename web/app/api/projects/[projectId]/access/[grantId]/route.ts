import { callCoreApi } from "@/lib/core-api";
import { handleRoute, requireId } from "@/lib/route-helpers";

type Params = { params: Promise<{ projectId: string; grantId: string }> };

export async function DELETE(request: Request, { params }: Params) {
  const { projectId, grantId } = await params;
  return handleRoute(request, () =>
    callCoreApi(`/projects/${requireId(projectId)}/access/${requireId(grantId)}`, {
      method: "DELETE",
    }),
  );
}
