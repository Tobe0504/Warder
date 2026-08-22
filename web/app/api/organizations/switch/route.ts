import { callCoreApi } from "@/lib/core-api";
import { handleRoute, readJson, requireString } from "@/lib/route-helpers";
import { startSession } from "@/lib/session";

type SwitchResponse = {
  sessionToken: string | null;
  expiresAt?: string;
  organization: string;
  slug: string;
  unchanged?: boolean;
};

/**
 * Moves this browser session to another organization the user belongs to.
 *
 * The core API mints a new session and revokes the old one, so the cookie has
 * to be replaced here or the very next request arrives carrying a dead token.
 * The new value never reaches the browser as data: it goes straight into the
 * httpOnly cookie, exactly as signing in does, and the response says only
 * which organization is now current.
 */
export async function POST(request: Request) {
  return handleRoute(request, async () => {
    const body = await readJson(request);

    const result = await callCoreApi<SwitchResponse>(
      "/auth/organizations/switch",
      {
        method: "POST",
        body: { organization: requireString(body, "organization", 128) },
      },
    );

    if (result.sessionToken && result.expiresAt) {
      await startSession(result.sessionToken, new Date(result.expiresAt));
    }

    return {
      organization: result.organization,
      slug: result.slug,
      unchanged: result.unchanged ?? false,
    };
  });
}
