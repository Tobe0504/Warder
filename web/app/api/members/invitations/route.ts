import { callCoreApi, CoreApiError } from "@/lib/core-api";
import { connection } from "@/lib/env";
import { handleRoute, optionalString, readJson, requireString } from "@/lib/route-helpers";

const ROLES = new Set(["OWNER", "ADMIN", "DEVELOPER", "VIEWER"]);

export async function GET(request: Request) {
  return handleRoute(request, () => callCoreApi("/members/invitations"));
}

/**
 * Issues an invitation and composes the link to send.
 *
 * The core API returns the bare token. Where this web application is reachable
 * is something only this layer knows, so the link is assembled here.
 *
 * The token goes in the URL **fragment**, not the path and not the query. A
 * fragment is never sent to a server, so the invitation cannot land in this
 * application's access log, a load balancer's, or a proxy's — and it cannot
 * escape through a Referer header to whatever the invitee clicks next. The
 * accept page reads it in the browser and posts it back in a request body.
 */
export async function POST(request: Request) {
  return handleRoute(request, async () => {
    const body = await readJson(request);

    const role = requireString(body, "role", 32);
    if (!ROLES.has(role)) {
      throw new CoreApiError(400, "invalid_request", "Choose a valid role.");
    }

    const created = await callCoreApi<{
      invitationId: string;
      email: string;
      role: string;
      expiresAt: string;
      token: string;
    }>("/members/invitations", {
      method: "POST",
      body: {
        name: requireString(body, "name", 128),
        email: requireString(body, "email", 320),
        role,
        expiresAt: optionalString(body, "expiresAt", 64),
      },
    });

    return {
      invitationId: created.invitationId,
      email: created.email,
      role: created.role,
      expiresAt: created.expiresAt,
      inviteUrl: `${connection().appOrigin}/invite#${created.token}`,
    };
  });
}
