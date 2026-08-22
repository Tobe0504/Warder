import { callCoreApi } from "@/lib/core-api";
import {
  handleRoute,
  optionalString,
  readJson,
  requireId,
} from "@/lib/route-helpers";

type Params = { params: Promise<{ secretId: string }> };

type Version = {
  id: string;
  version: number;
  status: string;
  revokedAt: string | null;
};

/**
 * Changes when a secret's current version expires.
 *
 * The core API takes a version id; the dashboard's secrets table carries only
 * the ordinal. Resolving that here rather than in the browser keeps it to one
 * round trip and means the page never has to reason about which version is
 * current, which is a question about the store, not about the view.
 *
 * An empty expiresAt clears the expiry, which is what the core API does with an
 * absent value. Authorization is the core API's to decide: this needs
 * ROTATE_SECRET, the same authority as minting a new version, because extending
 * an expiry keeps a live credential alive.
 */
export async function POST(request: Request, { params }: Params) {
  const { secretId } = await params;

  return handleRoute(request, async () => {
    const body = await readJson(request);
    const id = requireId(secretId);

    const { versions } = await callCoreApi<{ versions: Version[] | null }>(
      `/secrets/${id}/versions`,
    );

    // Highest version number that has not been revoked. The list is the
    // authority on what exists; picking here rather than trusting a number
    // from the browser means a stale page cannot retarget an old version.
    const current = (versions ?? [])
      .filter((version) => version.status === "ACTIVE" && !version.revokedAt)
      .sort((a, b) => b.version - a.version)[0];

    if (!current) {
      throw new Error("This secret has no active version to change.");
    }

    return callCoreApi(`/secrets/${id}/expiry`, {
      method: "POST",
      body: {
        versionId: current.id,
        expiresAt: optionalString(body, "expiresAt", 64),
      },
    });
  });
}
