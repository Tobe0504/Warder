import "server-only";

import { headers } from "next/headers";

import { connection } from "./env";
import { getSessionToken } from "./session";

export class CoreApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, string>;

  constructor(
    status: number,
    code: string,
    message: string,
    details?: Record<string, string>,
  ) {
    super(message);
    this.name = "CoreApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

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
  anonymous?: boolean;
  sessionToken?: string;
};

export async function callCoreApi<T>(
  path: string,
  options: CallOptions = {},
): Promise<T> {
  if (!path.startsWith("/") || path.startsWith("//")) {
    throw new Error(`Refusing to call a non-relative core API path: ${path}`);
  }

  const { coreApiUrl, serviceToken } = connection();

  const headers = new Headers({
    Accept: "application/json",
    "X-Service-Token": serviceToken,
  });

  if (!options.anonymous) {
    const token = options.sessionToken ?? (await getSessionToken());
    if (!token) {
      throw new NotAuthenticatedError();
    }
    headers.set("Authorization", `Bearer ${token}`);
  }

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
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.timeout(15_000),
    });
  } catch {
    throw new CoreApiError(
      502,
      "upstream_unavailable",
      "The service is temporarily unavailable.",
    );
  }

  if (response.status >= 300 && response.status < 400) {
    throw new CoreApiError(
      502,
      "upstream_unavailable",
      "The service is temporarily unavailable.",
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const payload = await response.json().catch(() => null);

  const error = (
    payload as {
      error?: {
        code?: string;
        message?: string;
        details?: Record<string, string>;
      };
    }
  )?.error;

  if (!response.ok) {
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
      if (options.anonymous) {
        throw new CoreApiError(
          401,
          "invalid_credentials",
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
