import { callCoreApi, NotAuthenticatedError } from "@/lib/core-api";
import { jsonResponse } from "@/lib/route-helpers";
import { getSessionToken } from "@/lib/session";
import type { SessionUser } from "@/lib/session-user";

/**
 * Who is signed in, for the public header.
 *
 * The landing page and the documentation are static HTML — they read no cookie,
 * which is what makes them cacheable and indexable. The account menu in the
 * header is the one part that depends on who is asking, so it asks here after
 * the page has loaded rather than making every public page dynamic.
 *
 * Signed out is not an error. It answers `{ user: null }` with a 200, because
 * "nobody is signed in" is the expected state for most visitors to a public
 * page, and a 401 would fill their console with failures that mean nothing.
 *
 * No CSRF token is required, and none is meaningful: this reads, it does not
 * act. The session cookie is SameSite=strict, so a request from another origin
 * arrives without it and is answered with `{ user: null }` — another site
 * cannot use this to learn anything about the visitor.
 */
export async function GET() {
  if (!(await getSessionToken())) {
    return jsonResponse({ user: null }, 200);
  }

  try {
    const user = await callCoreApi<SessionUser>("/auth/session");
    // Only what the header renders. The core API's answer is not forwarded
    // wholesale, so a field added upstream cannot start appearing in a browser
    // response without someone choosing it here.
    return jsonResponse(
      {
        user: {
          name: user.name,
          email: user.email,
          organization: user.organization,
        },
      },
      200,
    );
  } catch (error) {
    if (error instanceof NotAuthenticatedError) {
      return jsonResponse({ user: null }, 200);
    }
    // An unreachable core API should not break the public site. The header
    // falls back to its signed-out state, which is still usable.
    return jsonResponse({ user: null }, 200);
  }
}
