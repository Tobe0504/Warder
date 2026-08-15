import "server-only";

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
