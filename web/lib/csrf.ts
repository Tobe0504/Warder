import "server-only";

import { CSRF_HEADER, getCsrfToken } from "./session";
import { connection } from "./env";

/**
 * Cross-site request forgery protection for the BFF.
 *
 * Three independent checks guard every state-changing request, because each one
 * fails in a different situation:
 *
 *   SameSite=strict on the session cookie means a cross-site request arrives
 *   with no credential at all. This is the strongest of the three and handles
 *   the ordinary case entirely — but it depends on the browser, and older or
 *   unusual clients may not enforce it.
 *
 *   The Origin check refuses requests that announce a different origin. Browsers
 *   set Origin on every state-changing request and a page cannot forge it.
 *
 *   The double-submit token requires a value from a cookie to be echoed in a
 *   header. Only same-origin script can read that cookie, so an attacker's page
 *   cannot supply it even if it somehow got a credential attached.
 *
 * CORS is deliberately not part of this list. A permissive CORS policy would
 * weaken these, but a restrictive one is not itself a defence: simple requests
 * are sent before the response is inspected, so the damage is already done by
 * the time CORS refuses to show the attacker the answer.
 */

export class CsrfError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CsrfError";
  }
}

const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

/**
 * Verifies that a state-changing request came from this application's own
 * pages. Throws CsrfError otherwise.
 *
 * This is the half of the protection that works before a session exists, which
 * is why it is separable from the token check below.
 *
 * Sign-in needs it. Without an origin check, another site can forge a sign-in
 * POST carrying the attacker's own credentials — no cookie required, since the
 * victim has no session yet — and the browser ends up holding a valid session
 * for the *attacker's* organization. The victim then works in it, and any
 * secret they add lands somewhere the attacker can read. It is a quiet attack,
 * because everything appears to work.
 */
export async function assertSameOrigin(request: Request): Promise<void> {
  if (SAFE_METHODS.has(request.method.toUpperCase())) {
    return;
  }

  // A missing Origin on a state-changing request is refused rather than
  // allowed: every browser sends it for these methods, so its absence means the
  // caller is not a browser doing what we expect.
  const origin = request.headers.get("origin");
  if (!origin) {
    throw new CsrfError("This request is missing its origin.");
  }
  if (!originMatches(origin, request)) {
    throw new CsrfError("This request came from an unexpected origin.");
  }
}

/**
 * Verifies a request may change state on an existing session.
 *
 * Origin first, then the double-submit token. Both are required.
 */
export async function assertCsrf(request: Request): Promise<void> {
  if (SAFE_METHODS.has(request.method.toUpperCase())) {
    return;
  }

  await assertSameOrigin(request);

  const expected = await getCsrfToken();
  const presented = request.headers.get(CSRF_HEADER);

  if (!expected || !presented) {
    throw new CsrfError("This request is missing its security token.");
  }
  if (!timingSafeEqual(expected, presented)) {
    throw new CsrfError("This request carried an invalid security token.");
  }
}

function originMatches(origin: string, request: Request): boolean {
  const { appOrigin, isProduction } = connection();

  // The origin declared in WARDER_URL is authoritative. In production it is the
  // only thing accepted, so a deployment cannot be reached from an address it
  // did not declare.
  if (origin === appOrigin) {
    return true;
  }
  if (isProduction) {
    return false;
  }

  // Development also accepts the host the browser actually used, so that
  // localhost, 127.0.0.1, and a LAN address all work without anyone having to
  // configure each one.
  //
  // The comparison is against the Host header rather than request.url. Next.js
  // populates request.url from the server's own listening address, so a browser
  // on http://127.0.0.1:3000 produces a request.url of http://localhost:3000 —
  // and comparing the two rejects a perfectly ordinary local request with
  // "unexpected origin". That failure is indistinguishable from an attack to
  // whoever hits it, which is how origin checking ends up switched off.
  const host = request.headers.get("host");
  if (!host) {
    return false;
  }
  try {
    return new URL(origin).host === host;
  } catch {
    return false;
  }
}

/**
 * Compares two strings without leaking their contents through timing.
 *
 * Length is compared first and returns early, which does reveal whether the
 * lengths match. That is acceptable here: the token length is fixed and public.
 */
function timingSafeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) {
    return false;
  }
  let difference = 0;
  for (let i = 0; i < a.length; i++) {
    difference |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }
  return difference === 0;
}
