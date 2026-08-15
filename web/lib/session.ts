import "server-only";

import { cookies } from "next/headers";

import { connection } from "./env";

/**
 * Browser session handling.
 *
 * The session credential lives in an HttpOnly cookie and is never readable by
 * client JavaScript. A cross-site scripting flaw in the dashboard can therefore
 * act as the user while it runs — which is bad — but cannot exfiltrate the
 * session itself to be replayed later from somewhere else, which is worse.
 *
 * The CSRF token is a separate cookie that client script *can* read, because it
 * has to be echoed back in a header. It confers nothing on its own.
 */

export const SESSION_COOKIE = "warder_session";
export const CSRF_COOKIE = "warder_csrf";
export const CSRF_HEADER = "x-csrf-token";

/**
 * Whether cookies are marked Secure.
 *
 * Driven by the connection URI's posture rather than NODE_ENV, so that a
 * production build run against a development connection still behaves
 * consistently with the rest of the configuration. Getting this wrong in either
 * direction is bad: too strict and a local session silently never persists, too
 * loose and a session travels over plain HTTP.
 */
function secureCookies(): boolean {
  try {
    return connection().isProduction;
  } catch {
    // Unconfigured: assume the safer setting.
    return true;
  }
}

/** Reads the session credential, or null when signed out. */
export async function getSessionToken(): Promise<string | null> {
  const store = await cookies();
  return store.get(SESSION_COOKIE)?.value ?? null;
}

/** Reads the CSRF token the server issued for this session. */
export async function getCsrfToken(): Promise<string | null> {
  const store = await cookies();
  return store.get(CSRF_COOKIE)?.value ?? null;
}

/**
 * Stores a new session and a matching CSRF token.
 *
 * The attributes are the security-relevant part:
 *
 *   httpOnly   client script cannot read the session credential
 *   secure     it is never transmitted over plain HTTP in production
 *   sameSite   'strict' — the browser will not attach it to any cross-site
 *              request at all, which removes the request-forgery class before
 *              the token check even runs
 *   maxAge     matched to the server-side expiry, so the browser stops sending
 *              a credential the server would refuse anyway
 */
export async function startSession(token: string, expiresAt: Date): Promise<string> {
  const store = await cookies();
  const secure = secureCookies();
  const maxAge = Math.max(0, Math.floor((expiresAt.getTime() - Date.now()) / 1000));

  store.set(SESSION_COOKIE, token, {
    httpOnly: true,
    secure,
    sameSite: "strict",
    path: "/",
    maxAge,
  });

  const csrfToken = generateCsrfToken();
  store.set(CSRF_COOKIE, csrfToken, {
    // Deliberately readable by client script: it has to be sent back in a
    // header, and it is useless to an attacker who cannot also read it — which
    // the same-origin policy prevents.
    httpOnly: false,
    secure,
    sameSite: "strict",
    path: "/",
    maxAge,
  });

  return csrfToken;
}

/** Clears both cookies. */
export async function endSession(): Promise<void> {
  const store = await cookies();
  store.delete(SESSION_COOKIE);
  store.delete(CSRF_COOKIE);
}

function generateCsrfToken(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}
