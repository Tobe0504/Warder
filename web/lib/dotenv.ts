/**
 * Parses pasted `.env` text into key/value pairs.
 *
 * Deliberately not a dependency. The input is a file full of production
 * credentials, and the parsers on npm are larger than this, change more often
 * than this, and would be an unusually attractive package to compromise. The
 * format is small enough to own.
 *
 * What it handles, because real files contain all of it:
 *
 *   KEY=value                 plain
 *   export KEY=value          copied out of a shell profile
 *   KEY = value               spaces around the equals
 *   KEY="value with spaces"   double quoted, with \n and \" unescaped
 *   KEY='value'               single quoted, taken literally
 *   KEY=value # note          trailing comment, only on unquoted values
 *   KEY=                      present but empty
 *   # comment                 skipped
 *   KEY="line one             a quoted value spanning several lines
 *   line two"
 *
 * Values are returned exactly as written otherwise: no trimming of quoted
 * content, no interpolation of `$OTHER`, and no expansion of anything. A broker
 * that quietly rewrote a credential on the way in would be a bad broker.
 */

export type ParsedEntry = { key: string; value: string };

export type ParseResult = {
  entries: ParsedEntry[];
  /** Lines that looked like content but could not be read, with line numbers. */
  skipped: { line: number; text: string }[];
};

/** Matches what the API accepts, so the paste fails here rather than at the server. */
const VALID_KEY = /^[A-Za-z_][A-Za-z0-9_]*$/;

export function parseDotenv(input: string): ParseResult {
  const entries: ParsedEntry[] = [];
  const skipped: ParseResult["skipped"] = [];

  const lines = input.split(/\r?\n/);

  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i] ?? "";
    const line = raw.trim();

    if (line === "" || line.startsWith("#")) continue;

    // `export FOO=bar` is what you get from a shell profile.
    const withoutExport = line.replace(/^export\s+/, "");

    const equals = withoutExport.indexOf("=");
    if (equals === -1) {
      skipped.push({ line: i + 1, text: truncate(withoutExport) });
      continue;
    }

    const key = withoutExport.slice(0, equals).trim();
    if (!VALID_KEY.test(key)) {
      skipped.push({ line: i + 1, text: truncate(withoutExport) });
      continue;
    }

    let rest = withoutExport.slice(equals + 1).trim();

    const quote = rest[0];
    if (quote === '"' || quote === "'") {
      // A quoted value may run past the end of this line. Keep consuming
      // lines until the closing quote, so a PEM key or a JSON blob survives
      // the paste instead of arriving truncated.
      let body = rest.slice(1);
      let closed = closingIndex(body, quote) !== -1;

      while (!closed && i + 1 < lines.length) {
        i++;
        body += "\n" + (lines[i] ?? "");
        closed = closingIndex(body, quote) !== -1;
      }

      if (!closed) {
        skipped.push({ line: i + 1, text: truncate(key + "=…") });
        continue;
      }

      const end = closingIndex(body, quote);
      const value = body.slice(0, end);
      entries.push({
        key,
        // Escapes are only meaningful inside double quotes, which is how the
        // shell treats them and how these files are usually written.
        value: quote === '"' ? unescapeDouble(value) : value,
      });
      continue;
    }

    // Unquoted: a `#` starts a comment, but only when it follows whitespace.
    // Otherwise a value like `abc#123` would lose half of itself.
    const comment = rest.search(/\s#/);
    if (comment !== -1) rest = rest.slice(0, comment);

    entries.push({ key, value: rest.trim() });
  }

  return { entries, skipped };
}

/** Finds the closing quote, ignoring one escaped with a backslash. */
function closingIndex(body: string, quote: string): number {
  for (let i = 0; i < body.length; i++) {
    if (body[i] === "\\") {
      i++;
      continue;
    }
    if (body[i] === quote) return i;
  }
  return -1;
}

function unescapeDouble(value: string): string {
  return value
    .replace(/\\n/g, "\n")
    .replace(/\\r/g, "\r")
    .replace(/\\t/g, "\t")
    .replace(/\\"/g, '"')
    .replace(/\\\\/g, "\\");
}

/**
 * Shortens a line for an error message.
 *
 * A line that failed to parse may still be a credential, a bare token pasted
 * without a key, most likely, so only the first few characters are ever shown,
 * and never enough to use.
 */
function truncate(text: string): string {
  return text.length > 24 ? text.slice(0, 24) + "…" : text;
}

/**
 * Reports whether pasted text should be split into rows.
 *
 * Answered from the parser's own results rather than by a separate rule, since
 * a separate rule drifts from it. An earlier version required *every*
 * non-comment line to be an assignment, which sounded careful and rejected the
 * files people actually have: a `.env` holding a quoted PEM key has
 * continuation lines that are not assignments, and those are exactly the files
 * worth importing.
 *
 * The direction that matters is the other one: never shredding a single value
 * across rows. Yielding an entry is not enough on its own to prove a paste is a
 * file, because a bare PEM block does yield one: base64 padding makes
 * `MIIEow==` look exactly like a key, an equals sign and a value. What
 * separates the two is the company it keeps. A file is mostly assignments; a
 * value that happens to contain one is mostly not.
 */
export function looksLikeDotenv(text: string): boolean {
  const { entries, skipped } = parseDotenv(text);
  return entries.length > 0 && entries.length >= skipped.length;
}
