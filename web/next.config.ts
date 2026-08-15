import type { NextConfig } from "next";

/** Extracts the browser origin from WARDER_URL for the server-action guard. */
function allowedOrigin(): string {
  const raw = process.env.WARDER_URL;
  if (!raw) return "localhost:3000";
  try {
    const declared = new URL(raw).searchParams.get("origin");
    return declared ? new URL(declared).host : "localhost:3000";
  } catch {
    return "localhost:3000";
  }
}

const nextConfig: NextConfig = {
  reactStrictMode: true,

  // The version header names the framework and its version to anyone who asks.
  // It is free reconnaissance and buys nothing.
  poweredByHeader: false,

  // Security headers are set in middleware.ts, where the Content Security
  // Policy can carry a per-request nonce.

  experimental: {
    // Server actions accept only requests announcing this application's own
    // origin, which closes the same cross-site hole the BFF routes guard.
    //
    // Read directly from WARDER_URL rather than through lib/env, because this
    // file is evaluated by the Next CLI outside the application's module graph.
    // A malformed value falls back to localhost rather than throwing here,
    // since lib/connection.ts produces a far better error at request time.
    serverActions: {
      allowedOrigins: [allowedOrigin()],
    },
  },

  // The documentation pages read the repository's markdown, which lives outside
  // this directory. `generateStaticParams` means it is normally consumed at
  // build time and never needed again; this keeps the files reachable anyway,
  // so a deployment that falls back to rendering a docs page on demand serves
  // it instead of failing to find the source.
  outputFileTracingIncludes: {
    "/docs/[...slug]": ["../docs/**/*.md"],
  },

  typescript: {
    // A type error is a build failure. Shipping past one in a security tool is
    // not a trade worth making.
    ignoreBuildErrors: false,
  },
};

export default nextConfig;
