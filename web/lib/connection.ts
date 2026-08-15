/**
 * The Warder connection URI.
 *
 * This module is deliberately free of `server-only`, unlike its neighbours.
 * It is a pure parser: it reads no environment, holds no state, and reaches
 * nothing. That makes it directly testable, which matters more here than
 * elsewhere because a mistake in this file is a misconfiguration that reaches
 * production silently.
 *
 * The module that actually reads `process.env` is ./env.ts, and that one is
 * marked `server-only`. Nothing here can leak a credential on its own, because
 * nothing here has one until it is handed a string.
 *
 * The BFF needs four facts to work: where the core API is, the credential that
 * proves this application is the BFF, which posture to run in, and which origin
 * the browser will use. Previously each was its own environment variable, which
 * had two problems worth fixing.
 *
 * The operational one: four variables can be configured three-quarters of the
 * way. A deployment with a production address and a stale development token
 * starts up and fails at the first request, in a way that reads like a network
 * problem. Rotating the credential means touching one variable and hoping the
 * others still match.
 *
 * The security one follows from it. Configuration that is easy to get subtly
 * wrong gets subtly wrong, and "the token doesn't match the endpoint" is the
 * kind of wrong that ends with someone pasting a production token somewhere to
 * make an error go away.
 *
 * So there is one variable, in the shape everyone already knows from database
 * connection strings:
 *
 *   WARDER_URL=warder://<service-token>@<host>:<port>/<environment>?origin=<app-origin>
 *
 * Development, over plain HTTP on the loopback interface:
 *
 *   warder+insecure://dev-token@127.0.0.1:8080/development
 *
 * Production, over TLS:
 *
 *   warder://prod-token@warder-api.internal:8443/production?origin=https://vault.acme.com
 *
 * What this genuinely buys, stated precisely because the rest of this codebase
 * is careful about claims:
 *
 *   - One credential to store, rotate, and audit, instead of a set that can
 *     drift apart.
 *   - The token cannot be separated from the endpoint it authenticates to, so
 *     a development token cannot be left pointing at a production address.
 *   - The posture is part of the credential, so `warder+insecure://` reaching a
 *     production deployment is a startup failure rather than a quiet downgrade
 *     to unencrypted transport.
 *   - It either parses completely or the process refuses to start.
 *
 * What it does not buy: this is not cryptographically stronger than four
 * variables. It is harder to misconfigure, which in practice is where the
 * failures come from.
 *
 * The one risk it introduces is real and is handled explicitly. A URI carrying
 * a credential is easy to log by accident, because URLs are the most logged
 * kind of string there is. So the parsed value is never held as a raw string
 * anywhere it might be printed, `redacted()` exists for the cases where the
 * connection has to be described, and the Go logger scrubs this exact shape.
 */

/** How the BFF reaches the core API. */
export type Transport = "https" | "http";

/** Deployment posture, which gates the security defaults. */
export type Posture = "development" | "production";

export type Connection = {
  /** Origin of the core API, with no credential in it. Safe to use in fetch. */
  readonly coreApiUrl: string;
  /** The service credential. Never leaves the server. */
  readonly serviceToken: string;
  readonly posture: Posture;
  readonly transport: Transport;
  /** The origin the browser is expected to announce. */
  readonly appOrigin: string;
  readonly isProduction: boolean;
  /** A description safe to print, with the credential removed. */
  readonly redacted: string;
};

export class ConnectionError extends Error {
  constructor(message: string) {
    super(
      `${message}\n\n` +
        `WARDER_URL should look like:\n` +
        `  warder+insecure://<service-token>@127.0.0.1:8080/development\n` +
        `  warder://<service-token>@warder-api.internal:8443/production?origin=https://vault.example.com\n\n` +
        `Generate one with:  go run ./cmd/warder-api init`,
    );
    this.name = "ConnectionError";
  }
}

const MIN_TOKEN_LENGTH = 32;

/**
 * Parses and validates the connection URI.
 *
 * Every failure below throws. A BFF that starts with a half-understood
 * connection string is a BFF that will fail later, further from the cause.
 */
export function parseConnection(raw: string | undefined): Connection {
  if (!raw || raw.trim() === "") {
    throw new ConnectionError("WARDER_URL is not set.");
  }

  let url: URL;
  try {
    url = new URL(raw.trim());
  } catch {
    throw new ConnectionError("WARDER_URL is not a valid URI.");
  }

  // The scheme states the transport, so it cannot be inferred wrongly from a
  // port number or a hostname.
  let transport: Transport;
  switch (url.protocol) {
    case "warder:":
      transport = "https";
      break;
    case "warder+insecure:":
      transport = "http";
      break;
    default:
      throw new ConnectionError(
        `WARDER_URL uses an unrecognized scheme "${url.protocol.replace(":", "")}". ` +
          `Use "warder" for TLS, or "warder+insecure" for local development over plain HTTP.`,
      );
  }

  // The whole userinfo component is the service token. A password component
  // would mean the URI was written in the "user:password" shape, which is not
  // what this is, and silently accepting it would leave half the credential
  // behind.
  if (url.password !== "") {
    throw new ConnectionError(
      "WARDER_URL should carry the service token as the whole userinfo component, " +
        "with no colon: warder://<token>@host:port/environment",
    );
  }

  const serviceToken = decodeURIComponent(url.username);
  if (serviceToken === "") {
    throw new ConnectionError("WARDER_URL is missing the service token before the '@'.");
  }
  if (serviceToken.length < MIN_TOKEN_LENGTH) {
    // The length is safe to state; the token obviously is not.
    throw new ConnectionError(
      `The service token in WARDER_URL must be at least ${MIN_TOKEN_LENGTH} characters. ` +
        `It is the only thing standing between a network-adjacent caller and the core API.`,
    );
  }

  if (url.hostname === "") {
    throw new ConnectionError("WARDER_URL is missing the core API host.");
  }

  // The path is the posture. Anything else is refused rather than defaulted,
  // because defaulting this would mean a typo silently selects development
  // security settings in production.
  const posturePath = url.pathname.replace(/^\/+|\/+$/g, "");
  let posture: Posture;
  switch (posturePath) {
    case "development":
      posture = "development";
      break;
    case "production":
      posture = "production";
      break;
    case "":
      throw new ConnectionError(
        "WARDER_URL is missing the environment path. End it with /development or /production.",
      );
    default:
      throw new ConnectionError(
        `WARDER_URL has an unrecognized environment "${posturePath}". Use /development or /production.`,
      );
  }

  // Plain HTTP in production would send the service credential, and every
  // session it carries, in the clear. This is refused rather than warned about.
  if (posture === "production" && transport === "http") {
    throw new ConnectionError(
      "WARDER_URL uses warder+insecure (plain HTTP) with /production. " +
        "The service token and every session would travel unencrypted. Use warder:// for TLS.",
    );
  }

  const port = url.port !== "" ? `:${url.port}` : "";
  const coreApiUrl = `${transport}://${url.hostname}${port}`;

  const appOrigin = resolveAppOrigin(url, posture);

  return {
    coreApiUrl,
    serviceToken,
    posture,
    transport,
    appOrigin,
    isProduction: posture === "production",
    // The credential is replaced rather than truncated: a prefix of a secret is
    // still a piece of a secret.
    redacted: `${url.protocol}//[redacted]@${url.hostname}${port}/${posture}`,
  };
}

function resolveAppOrigin(url: URL, posture: Posture): string {
  const declared = url.searchParams.get("origin");

  if (declared) {
    let parsed: URL;
    try {
      parsed = new URL(declared);
    } catch {
      throw new ConnectionError(`The origin parameter in WARDER_URL is not a valid URL: ${declared}`);
    }
    if (parsed.protocol !== "https:" && posture === "production") {
      throw new ConnectionError(
        "The origin parameter must use https in production. Cookies are marked Secure and " +
          "would not be sent over plain HTTP.",
      );
    }
    // Origin only: a path or query here would never match a browser's Origin
    // header and would silently break the cross-site checks.
    return parsed.origin;
  }

  if (posture === "production") {
    throw new ConnectionError(
      "WARDER_URL must declare the browser origin in production, so that cross-site requests " +
        "can be refused: ?origin=https://vault.example.com",
    );
  }

  // Development only. The CSRF check additionally accepts the request's own
  // host in this posture, so localhost, 127.0.0.1, and a LAN address all work.
  return "http://localhost:3000";
}
