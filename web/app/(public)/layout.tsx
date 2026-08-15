import type { Metadata } from "next";

import { SiteFooter, SiteHeader } from "@/components/site-header";

/**
 * The public site: landing page and documentation.
 *
 * Separated from the dashboard by a route group because the two have opposite
 * requirements. Dashboard pages render secret metadata, must never be cached or
 * indexed, and are rendered per request. These pages contain nothing but
 * published prose, so they are static, cacheable and indexable — and they read
 * no session, which is what makes that safe.
 */
export const metadata: Metadata = {
  // Overrides the root layout, which keeps the dashboard out of search results.
  // Documentation is meant to be found.
  robots: { index: true, follow: true },
};

/**
 * Overrides the root layout's `force-dynamic`, which exists for the dashboard.
 * Nothing under this group depends on the request.
 */
export const dynamic = "force-static";

export default function PublicLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-dvh flex-col ">
      <SiteHeader />
      <div className="flex-1">{children}</div>
      <SiteFooter />
    </div>
  );
}
