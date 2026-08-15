import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { looksLikeDotenv, parseDotenv } from "./dotenv.ts";

describe("parseDotenv", () => {
  it("reads a plain assignment", () => {
    const { entries } = parseDotenv("DATABASE_URL=postgres://localhost/app");
    assert.deepEqual(entries, [{ key: "DATABASE_URL", value: "postgres://localhost/app" }]);
  });

  it("skips comments and blank lines", () => {
    const { entries } = parseDotenv("# a note\n\nAPI_KEY=abc\n\n# another\n");
    assert.deepEqual(entries, [{ key: "API_KEY", value: "abc" }]);
  });

  it("accepts the export prefix from a shell profile", () => {
    const { entries } = parseDotenv("export STRIPE_KEY=sk_test_123");
    assert.deepEqual(entries, [{ key: "STRIPE_KEY", value: "sk_test_123" }]);
  });

  it("tolerates spaces around the equals", () => {
    const { entries } = parseDotenv("KEY = value");
    assert.deepEqual(entries, [{ key: "KEY", value: "value" }]);
  });

  it("keeps spaces inside a double-quoted value", () => {
    const { entries } = parseDotenv('GREETING="hello there"');
    assert.deepEqual(entries, [{ key: "GREETING", value: "hello there" }]);
  });

  it("takes a single-quoted value literally", () => {
    const { entries } = parseDotenv("RAW='no \\n escape'");
    assert.deepEqual(entries, [{ key: "RAW", value: "no \\n escape" }]);
  });

  it("unescapes newlines inside double quotes", () => {
    const { entries } = parseDotenv('KEY="line one\\nline two"');
    assert.deepEqual(entries, [{ key: "KEY", value: "line one\nline two" }]);
  });

  it("strips a trailing comment from an unquoted value", () => {
    const { entries } = parseDotenv("PORT=5432 # the database port");
    assert.deepEqual(entries, [{ key: "PORT", value: "5432" }]);
  });

  it("does not treat a hash inside a value as a comment", () => {
    // A password of `p#ssw0rd` must survive intact, or the broker silently
    // stores a credential that does not work.
    const { entries } = parseDotenv("PASSWORD=p#ssw0rd");
    assert.deepEqual(entries, [{ key: "PASSWORD", value: "p#ssw0rd" }]);
  });

  it("keeps a hash inside a quoted value", () => {
    const { entries } = parseDotenv('URL="https://example.com/#anchor"');
    assert.deepEqual(entries, [{ key: "URL", value: "https://example.com/#anchor" }]);
  });

  it("reads a quoted value that spans several lines", () => {
    // The PEM case. Truncating this at the first newline would store a private
    // key that cannot be used and would not obviously look broken.
    const input = 'PRIVATE_KEY="-----BEGIN KEY-----\nMIIEow==\n-----END KEY-----"\nNEXT=after';
    const { entries } = parseDotenv(input);
    assert.equal(entries.length, 2);
    assert.equal(entries[0]?.key, "PRIVATE_KEY");
    assert.equal(entries[0]?.value, "-----BEGIN KEY-----\nMIIEow==\n-----END KEY-----");
    assert.deepEqual(entries[1], { key: "NEXT", value: "after" });
  });

  it("keeps an escaped quote inside a quoted value", () => {
    const { entries } = parseDotenv('JSON="{\\"a\\": 1}"');
    assert.deepEqual(entries, [{ key: "JSON", value: '{"a": 1}' }]);
  });

  it("keeps an equals sign that appears inside a value", () => {
    const { entries } = parseDotenv("TOKEN=abc=def==");
    assert.deepEqual(entries, [{ key: "TOKEN", value: "abc=def==" }]);
  });

  it("accepts a present but empty value", () => {
    const { entries } = parseDotenv("EMPTY=");
    assert.deepEqual(entries, [{ key: "EMPTY", value: "" }]);
  });

  it("reports lines it could not read instead of dropping them", () => {
    const { entries, skipped } = parseDotenv("GOOD=1\njust-some-text\nALSO_GOOD=2");
    assert.equal(entries.length, 2);
    assert.equal(skipped.length, 1);
    assert.equal(skipped[0]?.line, 2);
  });

  it("rejects a key that the API would reject", () => {
    const { entries, skipped } = parseDotenv("9INVALID=x\nVALID=y");
    assert.deepEqual(entries, [{ key: "VALID", value: "y" }]);
    assert.equal(skipped.length, 1);
  });

  it("never echoes a full line it could not parse", () => {
    const secret = "a".repeat(200);
    const { skipped } = parseDotenv(secret);
    assert.equal(skipped.length, 1);
    assert.ok(skipped[0]!.text.length < 30, "a skipped line must be truncated");
    assert.ok(!skipped[0]!.text.includes(secret));
  });

  it("handles CRLF line endings", () => {
    const { entries } = parseDotenv("A=1\r\nB=2\r\n");
    assert.deepEqual(entries, [
      { key: "A", value: "1" },
      { key: "B", value: "2" },
    ]);
  });
});

describe("looksLikeDotenv", () => {
  it("recognizes a file", () => {
    assert.equal(looksLikeDotenv("A=1\nB=2"), true);
  });

  it("recognizes a single assignment", () => {
    assert.equal(looksLikeDotenv("DATABASE_URL=postgres://x"), true);
  });

  it("does not mistake a connection string for a file", () => {
    // This contains an equals sign and is exactly what somebody pastes into a
    // single value field. Splitting it up would scatter one credential.
    assert.equal(looksLikeDotenv("postgres://user:pass@host:5432/db?sslmode=disable"), false);
  });

  it("does not mistake a bare token for a file", () => {
    assert.equal(looksLikeDotenv("sk_live_51H8xK2eZvKYlo2C"), false);
  });

  it("accepts a file with lines it cannot read, and reports them", () => {
    // Filling what parses and naming the rest beats refusing the whole paste.
    assert.equal(looksLikeDotenv("A=1\nplease use these values"), true);
    assert.equal(parseDotenv("A=1\nplease use these values").skipped.length, 1);
  });

  it("accepts a file whose value spans lines", () => {
    // The case the old every-line-must-be-an-assignment rule got wrong.
    assert.equal(looksLikeDotenv('KEY="line one\nline two"\nNEXT=2'), true);
  });

  it("does not mistake a PEM block for a file", () => {
    assert.equal(looksLikeDotenv("-----BEGIN KEY-----\nMIIEow==\n-----END KEY-----"), false);
  });

  it("ignores comments when deciding", () => {
    assert.equal(looksLikeDotenv("# staging\nA=1\nB=2"), true);
  });

  it("says no to empty input", () => {
    assert.equal(looksLikeDotenv("   \n\n"), false);
  });
});
