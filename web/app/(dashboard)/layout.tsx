import { redirect } from "next/navigation";

import { AppSidebar } from "@/components/app-sidebar";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { callCoreApi, NotAuthenticatedError } from "@/lib/core-api";
import { getSessionToken } from "@/lib/session";
import type { SessionUser } from "@/lib/session-user";

/**
 * Nothing under the dashboard may be pre-rendered or cached. Every page here
 * renders secret metadata scoped to one organization and one person, and the
 * session is re-validated on each render.
 */
export const dynamic = "force-dynamic";

// Re-exported for anything already importing the type from this route.
export type { SessionUser };

/**
 * The dashboard shell.
 *
 * The session is validated against the core API on every render rather than
 * trusted from the cookie. That is what makes revocation immediate: when a
 * membership expires or an administrator removes someone, the very next page
 * they load sends them to sign-in, without waiting for a cookie to lapse.
 *
 * If the core API is unreachable, this throws and app/(dashboard)/error.tsx
 * renders a page explaining what is not running. That path exists so nobody
 * ever has to comment out the check above to see the interface, which would
 * mean shipping a dashboard that renders without a verified session.
 */
export default async function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  if (!(await getSessionToken())) {
    redirect("/login");
  }

  let user: SessionUser;
  try {
    user = await callCoreApi<SessionUser>("/auth/session");
  } catch (error) {
    if (error instanceof NotAuthenticatedError) {
      redirect("/login");
    }
    throw error;
  }

  return (
    <TooltipProvider delayDuration={200}>
      <SidebarProvider>
        <AppSidebar user={user} />
        <SidebarInset>
          <a
            href="#main"
            className="sr-only focus:not-sr-only focus:absolute focus:left-3 focus:top-3 focus:z-50 focus:rounded-md focus:bg-card focus:px-3 focus:py-2 focus:text-meta focus:shadow-lg focus:outline-none focus:ring-2 focus:ring-ring"
          >
            Skip to content
          </a>
          {children}
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  );
}
