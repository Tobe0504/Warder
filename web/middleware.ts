import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

/**
 * Security headers for every response.
 *
 * These are set in middleware rather than in next.config so that the Content
 * Security Policy can carry a fresh nonce per request. A policy with
 * 'unsafe-inline' is close to no policy at all against the flaw it exists to
 * contain, and Next.js needs inline scripts for hydration, so a nonce is the
 * way to have both.
 */
/**
 * Next.js's development server compiles modules with `eval` for hot reloading.
 * A policy without 'unsafe-eval' therefore blocks every script on the page in
 * development: nothing hydrates, no form works, and the failure looks like the
 * application being broken rather than like a policy doing its job.
 *
 * So development gets 'unsafe-eval' and production does not. The condition is
 * on NODE_ENV, which Next sets itself and which is 'production' in any built
 * artifact — there is no configuration that can accidentally ship the relaxed
 * policy, and no reason for anyone to weaken the real one to get work done.
 */
const isDevelopment = process.env.NODE_ENV === "development";

export function middleware(request: NextRequest) {
  const nonce = generateNonce();

  const csp = [
    // Nothing loads unless a directive below says so.
    "default-src 'self'",

    // 'strict-dynamic' lets the nonce-carrying Next.js bootstrap load the
    // chunks it needs, without those chunks each needing a nonce. The
    // https: and 'unsafe-inline' entries are ignored by browsers that
    // understand strict-dynamic and act as a fallback for those that do not.
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic' https: 'unsafe-inline'${
      isDevelopment ? " 'unsafe-eval'" : ""
    }`,

    // Styles cannot use a nonce here: Next.js injects style tags during
    // development and some Radix primitives set inline styles for positioning.
    // This is the one relaxation in the policy, and it is a modest one —
    // style injection can reflow a page but cannot execute script.
    "style-src 'self' 'unsafe-inline'",

    "img-src 'self' data: blob:",
    "font-src 'self' data:",
    // The dashboard talks to its own origin and nowhere else, so an
    // exfiltration attempt to a third-party endpoint is refused by the browser.
    // Development additionally allows the websocket the dev server uses to push
    // reloads.
    isDevelopment ? "connect-src 'self' ws: wss:" : "connect-src 'self'",

    "object-src 'none'",
    "base-uri 'none'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "upgrade-insecure-requests",
  ].join("; ");

  // The policy is set on the *request* headers as well as the response.
  //
  // This is the part that is easy to get wrong and produces no visible symptom
  // when it is: Next.js reads the nonce out of the request's
  // Content-Security-Policy header in order to stamp it onto the script tags it
  // generates. Setting the header only on the response yields a page whose
  // policy names a nonce that none of its scripts carry — so either the app
  // silently fails to hydrate, or, if 'unsafe-inline' is present as a fallback,
  // the policy is quietly doing nothing at all.
  const headers = new Headers(request.headers);
  headers.set("x-nonce", nonce);
  headers.set("Content-Security-Policy", csp);

  const response = NextResponse.next({ request: { headers } });

  response.headers.set("Content-Security-Policy", csp);
  response.headers.set("X-Content-Type-Options", "nosniff");
  response.headers.set("X-Frame-Options", "DENY");
  response.headers.set("Referrer-Policy", "no-referrer");
  response.headers.set(
    "Permissions-Policy",
    "geolocation=(), camera=(), microphone=(), payment=(), usb=(), interest-cohort=()",
  );
  response.headers.set("Cross-Origin-Opener-Policy", "same-origin");
  response.headers.set("Cross-Origin-Resource-Policy", "same-origin");
  response.headers.set("X-DNS-Prefetch-Control", "off");

  // HTTP Strict Transport Security, set only over TLS so that a development
  // server on plain HTTP does not pin a browser to a scheme it cannot serve.
  if (request.nextUrl.protocol === "https:") {
    response.headers.set(
      "Strict-Transport-Security",
      "max-age=63072000; includeSubDomains; preload",
    );
  }

  // Every page in the dashboard renders secret metadata. None of it may be
  // retained by an intermediate cache or restored from the back-forward cache
  // after sign-out.
  //
  // The landing page and the documentation are the exception: they are static
  // published prose, they read no session, and they contain nothing that
  // depends on who is asking. Marking them no-store would make every visit
  // re-fetch pages that are identical for everyone. The list is an allowlist of
  // exact paths and one prefix, so a new dashboard route cannot fall into it by
  // resembling one of these.
  response.headers.set(
    "Cache-Control",
    isPublicPage(request.nextUrl.pathname)
      ? "public, max-age=0, must-revalidate"
      : "no-store, must-revalidate",
  );

  return response;
}

/**
 * Paths that carry no session-dependent content.
 *
 * The brand assets are here for the same reason as the pages: a favicon and a
 * logo are identical for every visitor, and re-fetching them on every
 * navigation because the dashboard's rule caught them is pure waste.
 */
function isPublicPage(pathname: string): boolean {
  if (pathname === "/" || pathname === "/docs" || pathname.startsWith("/docs/")) {
    return true;
  }
  return (
    pathname === "/manifest.webmanifest" ||
    /^\/(favicon|icon|apple-touch-icon|warder)-[\w-]+\.png$/.test(pathname)
  );
}

function generateNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return btoa(String.fromCharCode(...bytes));
}

export const config = {
  matcher: [
    // Everything except static assets, which carry no user data and would only
    // be slowed down by this.
    {
      source: "/((?!_next/static|_next/image|favicon.ico).*)",
      missing: [
        { type: "header", key: "next-router-prefetch" },
        { type: "header", key: "purpose", value: "prefetch" },
      ],
    },
  ],
};
