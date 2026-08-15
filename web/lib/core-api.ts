import "server-only";

import { headers } from "next/headers";

import { connection } from "./env";
import { getSessionToken } from "./session";

/**
 * The only path from this application to the core API.
 *
 * Every call names a fixed path built in code. There is deliberately no way to
 * pass a URL through from a request: a `/api/proxy?url=...` endpoint would let
 * anyone who can reach the dashboard make the server fetch arbitrary addresses,
 * including cloud metadata endpoints and internal services. That is server-side
 * request forgery, and the only reliable defence is not to build the primitive.
 *
 * The service credential is attached here, on the server, and never leaves it.
 */

export class CoreApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, string>;

  constructor(status: number, code: string, message: string, details?: Record<string, string>) {
    super(message);
    this.name = "CoreApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

/**
 * The address of the browser that caused this request, if there is one.
 *
 * Returns nothing outside a request, a build-time render, say, rather than
 * inventing an address, so the core API falls back to the connection it can
 * actually see.
 */
async function callerAddress(): Promise<string | null> {
  try {
    const incoming = await headers();
    const real = incoming.get("x-real-ip")?.trim();
    if (real) return real;

    const forwarded = incoming.get("x-forwarded-for");
    const first = forwarded?.split(",")[0]?.trim();
    return first || null;
  } catch {
    return null;
  }
}

export class NotAuthenticatedError extends CoreApiError {
  constructor() {
    super(401, "unauthorized", "Your session has ended. Sign in again.");
    this.name = "NotAuthenticatedError";
  }
}

type CallOptions = {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  /** Set for endpoints that run before a session exists, such as sign-in. */
  anonymous?: boolean;
  /** Overrides the session credential, used immediately after sign-in. */
  sessionToken?: string;
};

/**
 * Calls the core API.
 *
 * `path` must be a literal built at the call site. It is validated anyway,
 * because a defence that depends on every future caller being careful is not
 * one.
 */
export async function callCoreApi<T>(path: string, options: CallOptions = {}): Promise<T> {
  if (!path.startsWith("/") || path.startsWith("//")) {
    throw new Error(`Refusing to call a non-relative core API path: ${path}`);
  }

  const { coreApiUrl, serviceToken } = connection();

  const headers = new Headers({
    Accept: "application/json",
    // Proves to the core API that this request came from the BFF. Without it,
    // the core API refuses every request carrying a user session, which is what
    // stops a browser from calling it directly.
    "X-Service-Token": serviceToken,
  });

  if (!options.anonymous) {
    const token = options.sessionToken ?? (await getSessionToken());
    if (!token) {
      throw new NotAuthenticatedError();
    }
    headers.set("Authorization", `Bearer ${token}`);
  }

  // Forward the browser's address.
  //
  // Without this every request reaches the core API from one of Vercel's
  // egress addresses, which breaks two things quietly. Rate limits keyed by IP
  // become a single shared bucket, five sign-ins a minute across every user
  // on the deployment, and every audit entry records Vercel instead of the
  // person who acted.
  //
  // x-real-ip is preferred over x-forwarded-for: Vercel sets both, and
  // x-forwarded-for can also carry entries a client supplied. The core API
  // reads the left-most entry of the header it is given, so it is given
  // exactly one address and no list to be confused by.
  const clientIP = await callerAddress();
  if (clientIP) {
    headers.set("X-Forwarded-For", clientIP);
  }

  let body: string | undefined;
  if (options.body !== undefined) {
    body = JSON.stringify(options.body);
    headers.set("Content-Type", "application/json");
  }

  let response: Response;
  try {
    response = await fetch(`${coreApiUrl}${path}`, {
      method: options.method ?? "GET",
      headers,
      body,
      // Responses describe or carry credentials, so nothing here may be
      // retained by Next.js's data cache or served to a second user.
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.timeout(15_000),
    });
  } catch {
    // The underlying error can quote the internal address. It is replaced
    // rather than wrapped, so that the private topology does not reach a page.
    throw new CoreApiError(502, "upstream_unavailable", "The service is temporarily unavailable.");
  }

  // A redirect from the core API would send the service credential and the
  // user's session to whatever host the response names.
  if (response.status >= 300 && response.status < 400) {
    throw new CoreApiError(502, "upstream_unavailable", "The service is temporarily unavailable.");
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const payload = await response.json().catch(() => null);

  const error = (payload as { error?: { code?: string; message?: string; details?: Record<string, string> } })?.error;

  if (!response.ok) {
    // A rejected service credential is a deployment fault, not a signed-out
    // user. Treating it as the latter sends someone back to sign in, over and
    // over, while the real problem is that this application and the core API
    // disagree about WARDER_URL.
    if (error?.code === "service_unauthorized") {
      console.error(
        "The core API rejected this application's service credential. " +
          "The token in WARDER_URL must match WARDER_SERVICE_TOKEN on the core API. " +
          "Regenerate both with: go run ./cmd/warder-api init",
      );
      throw new CoreApiError(
        503,
        "misconfigured",
        "This dashboard is not configured correctly for its backend. Check the server logs.",
      );
    }

    if (response.status === 401) {
      // On an anonymous call there is no session to have ended. Sign-in
      // reaching here means the credentials were refused, and telling someone
      // their session expired while they are trying to start one sends them
      // looking for a problem that does not exist.
      if (options.anonymous) {
        throw new CoreApiError(
          401,
          "invalid_credentials",
          // Deliberately does not say which half was wrong: the core API
          // spends the same work either way so that a response cannot answer
          // "does this address have an account", and this message must not
          // give away what the timing does not.
          "That email and password do not match an account.",
        );
      }
      throw new NotAuthenticatedError();
    }

    throw new CoreApiError(
      response.status,
      error?.code ?? "request_failed",
      error?.message ?? "The request could not be completed.",
      error?.details,
    );
  }

  return payload as T;
}
