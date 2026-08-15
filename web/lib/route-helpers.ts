import "server-only";

import { NextResponse } from "next/server";
import { assertCsrf, CsrfError } from "./csrf";
import { CoreApiError, NotAuthenticatedError } from "./core-api";

/**
 * The shared shape of every BFF route.
 *
 * Each route validates the session, validates CSRF, calls the core API by a
 * fixed path, and shapes the response. Doing that through one wrapper means no
 * route can forget a step, and that error handling is uniform: an unexpected
 * failure becomes a generic message here rather than whatever the underlying
 * exception happened to say.
 */

type Handler = () => Promise<unknown>;

/**
 * The standard route: verify CSRF, run the handler, shape the response.
 *
 * Used by every route that acts on an existing session.
 */
export async function handleRoute(request: Request, handler: Handler): Promise<NextResponse> {
  try {
    await assertCsrf(request);
    const result = await handler();
    return jsonResponse(result ?? { ok: true }, 200);
  } catch (error) {
    return errorResponse(error);
  }
}

/**
 * The same shaping, without the CSRF check.
 *
 * Sign-in and organization creation run before a session exists, so there is no
 * double-submit token to verify, they enforce the origin check themselves.
 * They need this rather than handleRoute, because routing their failures back
 * through a wrapper that starts by checking CSRF would replace the real error
 * with "missing security token" and send whoever is debugging it somewhere
 * unrelated.
 */
export async function handlePreAuthRoute(handler: Handler): Promise<NextResponse> {
  try {
    const result = await handler();
    return jsonResponse(result ?? { ok: true }, 200);
  } catch (error) {
    return errorResponse(error);
  }
}

export function jsonResponse(body: unknown, status: number): NextResponse {
  const response = NextResponse.json(body, { status });
  // Belt and braces alongside the middleware: responses describing secrets are
  // never stored anywhere.
  response.headers.set("Cache-Control", "no-store, must-revalidate");
  return response;
}

function errorResponse(error: unknown): NextResponse {
  if (error instanceof NotAuthenticatedError) {
    return jsonResponse({ error: { code: "unauthorized", message: error.message } }, 401);
  }

  if (error instanceof CsrfError) {
    return jsonResponse({ error: { code: "csrf_failed", message: error.message } }, 403);
  }

  if (error instanceof CoreApiError) {
    // The core API's messages are already written for display; they are part of
    // its fixed catalogue and carry no internal detail.
    return jsonResponse(
      { error: { code: error.code, message: error.message, details: error.details } },
      error.status,
    );
  }

  // Anything else is unexpected. It is logged for an operator and replaced with
  // a generic message, because an unexpected exception's text is exactly the
  // kind of thing that quotes the values it was handling.
  console.error("Unhandled BFF error:", error);
  return jsonResponse(
    { error: { code: "internal_error", message: "Something went wrong. Try again." } },
    500,
  );
}

/**
 * Reads and validates a JSON body.
 *
 * Only the fields a route expects are read from the parsed object. Passing a
 * request body straight through to the core API would let a caller set fields
 * the form does not offer, which is how a "role" or "capabilities" field ends
 * up being chosen by the client.
 */
export async function readJson<T extends Record<string, unknown>>(request: Request): Promise<T> {
  try {
    const parsed = await request.json();
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      throw new Error("not an object");
    }
    return parsed as T;
  } catch {
    throw new CoreApiError(400, "bad_request", "The request could not be read.");
  }
}

/** Reads a required string field, trimmed. */
export function requireString(body: Record<string, unknown>, field: string, maxLength = 512): string {
  const value = body[field];
  if (typeof value !== "string" || value.trim() === "") {
    throw new CoreApiError(400, "invalid_request", `${field} is required.`);
  }
  const trimmed = value.trim();
  if (trimmed.length > maxLength) {
    throw new CoreApiError(400, "invalid_request", `${field} is too long.`);
  }
  return trimmed;
}

/** Reads an optional string field. */
export function optionalString(body: Record<string, unknown>, field: string, maxLength = 512): string {
  const value = body[field];
  if (value === undefined || value === null || value === "") {
    return "";
  }
  if (typeof value !== "string") {
    throw new CoreApiError(400, "invalid_request", `${field} is not valid.`);
  }
  const trimmed = value.trim();
  if (trimmed.length > maxLength) {
    throw new CoreApiError(400, "invalid_request", `${field} is too long.`);
  }
  return trimmed;
}

/** Reads a list of strings, bounded in length. */
export function stringList(body: Record<string, unknown>, field: string, maxItems = 64): string[] {
  const value = body[field];
  if (value === undefined || value === null) {
    return [];
  }
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string")) {
    throw new CoreApiError(400, "invalid_request", `${field} is not valid.`);
  }
  if (value.length > maxItems) {
    throw new CoreApiError(400, "invalid_request", `${field} has too many entries.`);
  }
  return value as string[];
}

/**
 * Validates an identifier taken from a URL segment before it is used to build a
 * core API path. This is what keeps a crafted path segment from redirecting the
 * upstream call somewhere else.
 */
const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function requireId(value: string): string {
  if (!UUID_PATTERN.test(value)) {
    throw new CoreApiError(404, "not_found", "The resource was not found.");
  }
  return value;
}
