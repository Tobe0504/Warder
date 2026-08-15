import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { ConnectionError, parseConnection } from "./connection.ts";

/**
 * Tests for the connection URI.
 *
 * Run with:  npm test
 *
 * These matter more than their size suggests. Every failure mode here is a
 * misconfiguration that would otherwise surface in production as something
 * else — a network error, an unexplained 401, or worst of all, a token sent
 * over plain HTTP without anyone noticing.
 */

const TOKEN = "a-service-token-that-is-long-enough-to-pass";

describe("parseConnection", () => {
  it("parses a development connection", () => {
    const c = parseConnection(`warder+insecure://${TOKEN}@127.0.0.1:8080/development`);

    assert.equal(c.coreApiUrl, "http://127.0.0.1:8080");
    assert.equal(c.serviceToken, TOKEN);
    assert.equal(c.posture, "development");
    assert.equal(c.transport, "http");
    assert.equal(c.isProduction, false);
    assert.equal(c.appOrigin, "http://localhost:3000");
  });

  it("parses a production connection", () => {
    const c = parseConnection(
      `warder://${TOKEN}@warder-api.internal:8443/production?origin=https://vault.acme.com`,
    );

    assert.equal(c.coreApiUrl, "https://warder-api.internal:8443");
    assert.equal(c.posture, "production");
    assert.equal(c.transport, "https");
    assert.equal(c.isProduction, true);
    assert.equal(c.appOrigin, "https://vault.acme.com");
  });

  it("omits the port when none is given", () => {
    const c = parseConnection(
      `warder://${TOKEN}@warder-api.internal/production?origin=https://vault.acme.com`,
    );
    assert.equal(c.coreApiUrl, "https://warder-api.internal");
  });

  // The whole point of folding four variables into one: the description that
  // gets logged must not carry the credential.
  it("never exposes the token in its redacted form", () => {
    const c = parseConnection(`warder+insecure://${TOKEN}@127.0.0.1:8080/development`);

    assert.ok(!c.redacted.includes(TOKEN), "redacted form contains the token");
    assert.ok(!c.redacted.includes(TOKEN.slice(0, 12)), "redacted form contains a token prefix");
    assert.ok(c.redacted.includes("127.0.0.1"), "redacted form should keep the host");
    assert.ok(c.redacted.includes("[redacted]"));
  });

  // Plain HTTP in production would put the service token and every session it
  // carries on the wire in the clear.
  it("refuses plain HTTP in production", () => {
    assert.throws(
      () => parseConnection(`warder+insecure://${TOKEN}@api.internal:8080/production`),
      (error: unknown) => {
        assert.ok(error instanceof ConnectionError);
        assert.match(error.message, /unencrypted|TLS/i);
        return true;
      },
    );
  });

  it("requires an https origin in production", () => {
    assert.throws(
      () => parseConnection(`warder://${TOKEN}@api.internal/production?origin=http://vault.acme.com`),
      ConnectionError,
    );
  });

  // Defaulting this would mean a typo silently selects development security
  // settings in a production deployment.
  it("refuses an unrecognized or missing posture", () => {
    for (const url of [
      `warder://${TOKEN}@api.internal/staging`,
      `warder://${TOKEN}@api.internal/`,
      `warder://${TOKEN}@api.internal`,
      `warder://${TOKEN}@api.internal/Production`,
    ]) {
      assert.throws(() => parseConnection(url), ConnectionError, `accepted ${url}`);
    }
  });

  it("requires an explicit browser origin in production", () => {
    assert.throws(
      () => parseConnection(`warder://${TOKEN}@api.internal/production`),
      (error: unknown) => {
        assert.ok(error instanceof ConnectionError);
        assert.match(error.message, /origin/i);
        return true;
      },
    );
  });

  it("refuses a short service token", () => {
    assert.throws(() => parseConnection("warder+insecure://short@127.0.0.1:8080/development"), ConnectionError);
  });

  it("refuses a missing service token", () => {
    for (const url of [
      "warder+insecure://@127.0.0.1:8080/development",
      "warder+insecure://127.0.0.1:8080/development",
    ]) {
      assert.throws(() => parseConnection(url), ConnectionError, `accepted ${url}`);
    }
  });

  // A "user:password" shape means the URI was written against a different
  // mental model, and accepting it would silently drop half the credential.
  it("refuses a user:password style userinfo", () => {
    assert.throws(
      () => parseConnection(`warder+insecure://user:${TOKEN}@127.0.0.1:8080/development`),
      ConnectionError,
    );
  });

  it("refuses an unknown scheme", () => {
    for (const url of [
      `https://${TOKEN}@127.0.0.1:8080/development`,
      `warder+http://${TOKEN}@127.0.0.1:8080/development`,
      `postgres://${TOKEN}@127.0.0.1:8080/development`,
    ]) {
      assert.throws(() => parseConnection(url), ConnectionError, `accepted ${url}`);
    }
  });

  it("refuses empty or malformed input", () => {
    for (const url of [undefined, "", "   ", "not a uri", "warder://"]) {
      assert.throws(() => parseConnection(url), ConnectionError, `accepted ${String(url)}`);
    }
  });

  // A token containing URI-reserved characters has to survive the round trip,
  // or a perfectly good generated token fails to authenticate for reasons that
  // look like a server problem.
  it("decodes a percent-encoded token", () => {
    const awkward = "token/with+reserved=characters&and-more-padding-here";
    const c = parseConnection(
      `warder+insecure://${encodeURIComponent(awkward)}@127.0.0.1:8080/development`,
    );
    assert.equal(c.serviceToken, awkward);
  });

  // The origin is compared against a browser's Origin header, which never
  // includes a path.
  it("reduces the origin parameter to a bare origin", () => {
    const c = parseConnection(
      `warder://${TOKEN}@api.internal/production?origin=https://vault.acme.com/dashboard?x=1`,
    );
    assert.equal(c.appOrigin, "https://vault.acme.com");
  });

  it("explains how to fix a bad value", () => {
    try {
      parseConnection("nonsense");
      assert.fail("should have thrown");
    } catch (error) {
      assert.ok(error instanceof ConnectionError);
      assert.match(error.message, /warder\+insecure:\/\//);
      assert.match(error.message, /warder-api init/);
    }
  });
});
