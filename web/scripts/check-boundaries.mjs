#!/usr/bin/env node
/**
 * Static checks on the client/server boundary.
 *
 * The `server-only` package already fails the build if a client component
 * imports a server module, and `npm run build` is the authoritative check. This
 * script exists because it runs in under a second and says exactly what is
 * wrong, which means the mistake gets caught while it is still being made
 * rather than three minutes later in a wall of bundler output.
 *
 * Run with: npm run check
 */

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve, dirname } from "node:path";

const root = resolve(import.meta.dirname, "..");

/** Modules that must never end up in a browser bundle. */
const SERVER_ONLY = [
  "lib/env",
  "lib/core-api",
  "lib/session",
  "lib/csrf",
  "lib/route-helpers",
  // Reads the repository's markdown from disk at build time. Harmless in
  // itself, but a client component importing it would pull node:fs into the
  // browser bundle and fail confusingly.
  "lib/docs",
];

const problems = [];

function walk(dir) {
  const files = [];
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === ".next" || entry === "scripts") continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      files.push(...walk(full));
    } else if (/\.(ts|tsx)$/.test(entry)) {
      files.push(full);
    }
  }
  return files;
}

const files = walk(root);

/**
 * Strips comments before scanning.
 *
 * Several of these forbidden names appear in comments that exist precisely to
 * say "we do not do this here". Matching those would train everyone to ignore
 * the checker, which is worse than not having one.
 */
function stripComments(source) {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, "") // block comments, including JSDoc
    .replace(/^\s*\/\/.*$/gm, "") // whole-line comments
    .replace(/([^:"'`])\/\/.*$/gm, "$1"); // trailing comments, leaving URLs alone
}

// ---------------------------------------------------------------------------
// 1. No NEXT_PUBLIC_ anywhere.
//
// Anything under that prefix is inlined into the browser bundle at build time.
// None of this application's configuration belongs there: the browser talks
// only to this application and needs no address, no token, and no key.
// ---------------------------------------------------------------------------
for (const file of files) {
  const source = stripComments(readFileSync(file, "utf8"));
  if (source.includes("NEXT_PUBLIC_")) {
    problems.push(`${relative(root, file)}: uses NEXT_PUBLIC_, which ships to the browser`);
  }
}

// ---------------------------------------------------------------------------
// 2. No client component reaches a server-only module.
//
// Checked transitively: a client component importing a helper that imports
// lib/core-api is just as broken as importing it directly, and is harder to
// spot by reading.
// ---------------------------------------------------------------------------
const sources = new Map(files.map((file) => [file, stripComments(readFileSync(file, "utf8"))]));

function importsOf(file) {
  const source = sources.get(file) ?? "";
  const found = [];
  for (const match of source.matchAll(/from\s+["']([^"']+)["']/g)) {
    const specifier = match[1];
    let resolved;
    if (specifier.startsWith("@/")) {
      resolved = join(root, specifier.slice(2));
    } else if (specifier.startsWith(".")) {
      resolved = resolve(dirname(file), specifier);
    } else {
      continue; // a package, not ours
    }
    for (const candidate of [
      `${resolved}.ts`,
      `${resolved}.tsx`,
      join(resolved, "index.ts"),
      join(resolved, "index.tsx"),
    ]) {
      if (sources.has(candidate)) {
        found.push(candidate);
        break;
      }
    }
  }
  return found;
}

function isServerOnly(file) {
  const path = relative(root, file).replace(/\.(ts|tsx)$/, "");
  return SERVER_ONLY.includes(path);
}

for (const [file, source] of sources) {
  if (!/^\s*["']use client["']/m.test(source)) continue;

  const seen = new Set();
  const queue = [[file, []]];

  while (queue.length > 0) {
    const [current, path] = queue.shift();
    if (seen.has(current)) continue;
    seen.add(current);

    if (current !== file && isServerOnly(current)) {
      const chain = [...path, current].map((p) => relative(root, p)).join(" → ");
      problems.push(`${relative(root, file)}: client component reaches a server module: ${chain}`);
      break;
    }

    for (const next of importsOf(current)) {
      queue.push([next, [...path, current]]);
    }
  }
}

// ---------------------------------------------------------------------------
// 3. No dangerouslySetInnerHTML.
//
// Every value this interface renders is user-controlled: project names, secret
// keys, audit metadata. React escapes them, and this is the one API that opts
// out of that. If it is ever genuinely needed, it should arrive with a
// sanitizer and a comment explaining the threat model.
// ---------------------------------------------------------------------------
for (const [file, source] of sources) {
  if (source.includes("dangerouslySetInnerHTML")) {
    problems.push(`${relative(root, file)}: uses dangerouslySetInnerHTML`);
  }
}

// ---------------------------------------------------------------------------
// 4. No credentials in browser storage.
// ---------------------------------------------------------------------------
for (const [file, source] of sources) {
  for (const api of ["localStorage", "sessionStorage", "indexedDB"]) {
    if (source.includes(api)) {
      problems.push(`${relative(root, file)}: uses ${api}; secrets and sessions must not be stored there`);
    }
  }
}

if (problems.length > 0) {
  console.error("Boundary check failed:\n");
  for (const problem of problems) console.error(`  ✗ ${problem}`);
  console.error("");
  process.exit(1);
}

console.log(`Boundary check passed (${files.length} files).`);
