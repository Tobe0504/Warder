import { NextResponse } from "next/server";

import { cliRuntimeUrl } from "@/lib/env";

/**
 * Where the ward CLI should look for this Warder.
 *
 * A deployment has two hosts, and only one of them is the CLI's. Asking a
 * developer to know which is asking them to learn our topology to run their own
 * application, so this lets them use the one address they already have: the
 * dashboard they are signed in to.
 *
 * Deliberately unauthenticated, and it discloses nothing. The runtime host is
 * an address, not a credential; it answers `unauthorized` to anyone without a
 * token, and anyone who can read this can equally read the hostname out of the
 * setup commands on any environment page.
 *
 * Modelled on OIDC discovery, down to the shape: a stable well-known path that
 * names the endpoints a client needs.
 */
export const dynamic = "force-dynamic";

export function GET() {
  const runtimeUrl = cliRuntimeUrl();

  if (!runtimeUrl) {
    // Not configured. A 404 lets the CLI fall back to the address it was given
    // rather than treating a bare dashboard URL as an error.
    return NextResponse.json(
      { error: "runtime_url_not_configured" },
      { status: 404, headers: { "Cache-Control": "no-store" } },
    );
  }

  return NextResponse.json(
    { runtimeUrl },
    {
      headers: {
        // Short, not immutable: an operator who moves the runtime service
        // should not be fighting a cached answer for a day.
        "Cache-Control": "public, max-age=300",
      },
    },
  );
}
