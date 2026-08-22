import "server-only";

import { parseConnection, type Connection } from "./connection";

/**
 * Server-only configuration.
 *
 * The `server-only` import at the top is load-bearing. If any client component
 * ever imports this module, even transitively, the build fails rather than
 * quietly shipping the core API's address and service credential to the
 * browser.
 *
 * Everything the BFF needs comes from one variable, WARDER_URL. See
 * ./connection.ts for the format and the reasoning.
 *
 * Note what is absent from this file and from the entire web application: any
 * `NEXT_PUBLIC_` variable. The browser needs none of this, because the browser
 * talks only to this application. CI fails the build if one appears.
 */

let cached: Connection | null = null;

/**
 * Returns the parsed connection, parsing once per process.
 *
 * This is a function rather than a module-level constant so that a
 * configuration error surfaces as a handled error on the first request: which
 * renders a readable page, instead of as a module evaluation failure during
 * the build, which does not.
 */
export function connection(): Connection {
  if (cached === null) {
    cached = parseConnection(process.env.WARDER_URL);
  }
  return cached;
}

/**
 * Reports whether the application is configured, without throwing.
 *
 * Used by the setup page to tell a developer what is wrong on their first run,
 * rather than showing them a stack trace.
 */
export function configurationProblem(): string | null {
  try {
    connection();
    return null;
  } catch (error) {
    return error instanceof Error ? error.message : "WARDER_URL is not valid.";
  }
}

/**
 * The origin this application is served from, for absolute URLs in metadata.
 *
 * Social cards need an absolute image URL, and the only place this application
 * is told its own public address is the `origin` parameter of WARDER_URL. That
 * parameter is optional outside production, so this falls back to localhost and
 * never throws: a missing origin should degrade a preview card, not fail a
 * build or take down every page that renders metadata.
 */
export function publicOrigin(): string {
  try {
    return connection().appOrigin;
  } catch {
    return "http://localhost:3000";
  }
}

/**
 * The runtime address this dashboard advertises to the ward CLI.
 *
 * Served at /.well-known/warder and rendered into the setup commands. Not a
 * credential, and deliberately not a NEXT_PUBLIC variable: it reaches the
 * browser as a prop from a server component, which keeps the "no client-side
 * configuration" rule intact for a value that is only ever printed into a
 * copyable command.
 *
 * Absent means the dashboard shows a bare `ward init`, which is right for a
 * developer running Warder on their own machine and wrong for everyone using a
 * deployed one. Operators should set it. See docs/deployment.md.
 */
export function cliRuntimeUrl(): string | null {
  const raw = process.env.WARDER_PUBLIC_RUNTIME_URL?.trim();
  if (!raw) return null;

  try {
    const parsed = new URL(raw);
    if (parsed.protocol !== "https:" && parsed.protocol !== "http:") return null;
    // Userinfo in this value would be an operator mistake, and it would be
    // rendered into a command people copy and paste into shell history.
    parsed.username = "";
    parsed.password = "";
    return parsed.toString().replace(/\/$/, "");
  } catch {
    return null;
  }
}
