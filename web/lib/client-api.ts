/**
 * The browser's only way to reach the server.
 *
 * Every request goes to this application's own origin. There is no base URL to
 * configure, because the browser has no knowledge of the core API's address:
 * that is the point of the Backend-for-Frontend, and it is why nothing in this
 * file reads an environment variable.
 */

const CSRF_COOKIE = "warder_csrf";
const CSRF_HEADER = "x-csrf-token";

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, string>;

  constructor(status: number, code: string, message: string, details?: Record<string, string>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

function readCsrfToken(): string {
  const match = document.cookie.match(new RegExp(`(?:^|; )${CSRF_COOKIE}=([^;]*)`));
  return match?.[1] ? decodeURIComponent(match[1]) : "";
}

/**
 * Calls a BFF route.
 *
 * The CSRF token is attached automatically so that no caller has to remember,
 * and `credentials: "same-origin"` keeps the session cookie from being sent
 * anywhere but here.
 */
export async function apiFetch<T>(
  path: string,
  options: { method?: string; body?: unknown } = {},
): Promise<T> {
  const method = options.method ?? "GET";
  const headers: Record<string, string> = { Accept: "application/json" };

  if (method !== "GET" && method !== "HEAD") {
    headers[CSRF_HEADER] = readCsrfToken();
  }
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  const response = await fetch(path, {
    method,
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    credentials: "same-origin",
    cache: "no-store",
  });

  if (response.status === 401) {
    // The session ended. A full navigation clears any state still held in
    // memory, which for this application may include a revealed value.
    window.location.href = "/login";
    throw new ApiError(401, "unauthorized", "Your session has ended.");
  }

  const payload = response.status === 204 ? null : await response.json().catch(() => null);

  if (!response.ok) {
    const error = (payload as { error?: { code?: string; message?: string; details?: Record<string, string> } })
      ?.error;
    throw new ApiError(
      response.status,
      error?.code ?? "request_failed",
      error?.message ?? "The request could not be completed.",
      error?.details,
    );
  }

  return payload as T;
}
