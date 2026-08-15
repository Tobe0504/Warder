import "server-only";

import { readFile } from "node:fs/promises";
import { join } from "node:path";

/**
 * The documentation, read from the repository's own `docs/` directory.
 *
 * The markdown files are the source of truth and stay readable in a terminal,
 * in a pull request diff, and on this site. Nothing is duplicated into a CMS,
 * so a doc cannot drift from the version that ships with the code.
 *
 * Everything here runs at build time. `generateStaticParams` enumerates the
 * manifest below, the pages render to static HTML, and the `docs/` directory is
 * not needed at run time — which also means this public surface never reads a
 * session, a cookie, or the core API.
 */

/** Where the markdown lives, relative to the Next.js working directory. */
const DOCS_ROOT = join(process.cwd(), "..", "docs");

export type DocMeta = {
  /** URL path segment(s), e.g. "security/limitations". */
  slug: string;
  /** Path on disk, relative to the docs root. */
  file: string;
  /** Title shown in navigation. Overrides the file's own H1. */
  title: string;
  /** One line describing what the reader gets. Shown in listings. */
  summary: string;
};

export type DocSection = {
  title: string;
  /** Why this group exists, shown on the documentation index. */
  blurb: string;
  docs: DocMeta[];
};

/**
 * The reading order.
 *
 * Deliberately hand-written rather than derived from the filesystem. Directory
 * listings sort alphabetically, which would open the documentation on "audit"
 * and bury the page most people actually need. The order here is the order
 * someone adopting the product needs things in.
 */
export const DOC_SECTIONS: DocSection[] = [
  {
    title: "Start here",
    blurb: "What the product does, and what changes in your application.",
    docs: [
      {
        slug: "using-warder",
        file: "using-warder.md",
        title: "Using Warder in your application",
        summary:
          "What stays in your .env, where the two remaining variables come from, and how frontends fit.",
      },
      {
        slug: "developer-guide",
        file: "developer-guide.md",
        title: "Developer guide",
        summary: "Every command you need, in the order you need them.",
      },
    ],
  },
  {
    title: "How it works",
    blurb: "The design, and the reasoning behind it.",
    docs: [
      {
        slug: "architecture",
        file: "architecture/overview.md",
        title: "Architecture overview",
        summary: "The pieces, the two HTTP surfaces, and how a secret travels.",
      },
    ],
  },
  {
    title: "Security",
    blurb:
      "The part worth reading before you rely on this. Including what it does not do.",
    docs: [
      {
        slug: "security/threat-model",
        file: "security/threat-model.md",
        title: "Threat model",
        summary: "What is protected, from whom, and how each defence is verified.",
      },
      {
        slug: "security/limitations",
        file: "security/limitations.md",
        title: "Limitations",
        summary: "What Warder does not protect against, stated plainly.",
      },
      {
        slug: "security/key-management",
        file: "security/key-management.md",
        title: "Key management",
        summary: "The key everything depends on: where it lives and how it rotates.",
      },
      {
        slug: "security/audit",
        file: "security/audit.md",
        title: "Audit",
        summary: "What is recorded, what is never recorded, and why.",
      },
      {
        slug: "security/disaster-recovery",
        file: "security/disaster-recovery.md",
        title: "Disaster recovery",
        summary: "Restoring the two things that must be stored apart.",
      },
    ],
  },
];

/** Every document, flattened into reading order. */
export const ALL_DOCS: DocMeta[] = DOC_SECTIONS.flatMap((section) => section.docs);

export function findDoc(slug: string): DocMeta | undefined {
  return ALL_DOCS.find((doc) => doc.slug === slug);
}

/** The previous and next document, for continuous reading. */
export function docNeighbours(slug: string): { previous?: DocMeta; next?: DocMeta } {
  const index = ALL_DOCS.findIndex((doc) => doc.slug === slug);
  if (index === -1) return {};
  return { previous: ALL_DOCS[index - 1], next: ALL_DOCS[index + 1] };
}

export type Heading = { id: string; text: string; level: 2 | 3 };

export type LoadedDoc = {
  meta: DocMeta;
  /** The markdown body, with the leading H1 removed. */
  body: string;
  /** Second- and third-level headings, for the on-page contents. */
  headings: Heading[];
};

/**
 * Turns heading text into an anchor id.
 *
 * Matches GitHub's algorithm closely enough that the hand-written contents
 * lists already inside several of these documents keep working: lowercase,
 * drop anything that is not alphanumeric or a hyphen, and join words with
 * hyphens. "Worked example: a Next.js application" becomes
 * "worked-example-a-nextjs-application".
 */
export function slugifyHeading(text: string): string {
  return text
    .toLowerCase()
    // Underscores survive. Half the headings in these documents name an
    // environment variable — WARDER_TOKEN, WARDER_RUNTIME_ADDR — and stripping
    // the underscore turns them into a different word in the contents rail.
    .replace(/[^\p{L}\p{N}_ -]/gu, "")
    .trim()
    .replace(/\s+/g, "-");
}

/** Reads a document and prepares it for rendering. */
export async function loadDoc(slug: string): Promise<LoadedDoc | null> {
  const meta = findDoc(slug);
  if (!meta) return null;

  let raw: string;
  try {
    raw = await readFile(join(DOCS_ROOT, meta.file), "utf8");
  } catch {
    // A manifest entry pointing at a file that no longer exists should fail the
    // build loudly rather than render a blank page in production.
    throw new Error(
      `docs manifest names ${meta.file}, which could not be read from ${DOCS_ROOT}`,
    );
  }

  return { meta, body: stripLeadingH1(raw), headings: extractHeadings(raw) };
}

/**
 * Removes the document's own H1.
 *
 * The page renders the title itself, in the page header alongside the summary
 * and the section it belongs to. Leaving the H1 in the body would print it
 * twice.
 */
function stripLeadingH1(markdown: string): string {
  return markdown.replace(/^#\s+.*(\r?\n)+/, "");
}

/**
 * Collects headings for the contents rail.
 *
 * Fenced code blocks are tracked and skipped, because a shell comment at the
 * start of a line is indistinguishable from a heading otherwise — and several
 * of these documents contain exactly that.
 */
function extractHeadings(markdown: string): Heading[] {
  const headings: Heading[] = [];
  let insideFence = false;

  for (const line of markdown.split(/\r?\n/)) {
    if (/^\s*(```|~~~)/.test(line)) {
      insideFence = !insideFence;
      continue;
    }
    if (insideFence) continue;

    const match = /^(#{2,3})\s+(.+?)\s*#*\s*$/.exec(line);
    if (!match) continue;

    const [, hashes = "", raw = ""] = match;
    // Only the emphasis and code markers go. An underscore here is part of a
    // variable name, not markdown syntax.
    const text = raw.replace(/[`*]/g, "");
    headings.push({ id: slugifyHeading(text), text, level: hashes.length === 2 ? 2 : 3 });
  }

  return headings;
}
