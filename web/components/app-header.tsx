import Link from "next/link";

import { NavLink } from "@/components/nav-link";
import { SignOutButton } from "@/components/sign-out-button";
import type { SessionUser } from "@/app/(dashboard)/layout";

/**
 * The application header.
 *
 * Navigation is organization-level: projects, the identities that can use
 * secrets, and the audit trail. Project-scoped navigation lives inside a
 * project, so the top bar stays the same shape wherever you are.
 */
export function AppHeader({ user }: { user: SessionUser }) {
  return (
    <>
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-3 focus:top-3 focus:z-50 focus:rounded-md focus:bg-card focus:px-3 focus:py-2 focus:text-meta focus:shadow-lg focus:outline-none focus:ring-2 focus:ring-ring"
      >
        Skip to content
      </a>

      <header className="sticky top-0 z-40 border-b bg-background/85 backdrop-blur">
        <div className="mx-auto flex h-12 max-w-7xl items-center gap-4 px-4">
          <Link
            href="/projects"
            className="flex items-center gap-1.5 rounded-md text-body font-medium tracking-tight focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ShieldMark />
            Warder
          </Link>

          <nav
            className="flex items-center gap-0.5 text-meta"
            aria-label="Main"
          >
            <NavLink href="/projects">Projects</NavLink>
            <NavLink href="/identities">Identities</NavLink>
            <NavLink href="/audit">Audit</NavLink>
          </nav>

          <div className="ml-auto flex items-center gap-3">
            <div className="hidden text-right sm:block">
              <div className="text-meta leading-tight">{user.name}</div>
              <div className="text-meta leading-tight text-muted-foreground">
                {user.organization} · {user.role.toLowerCase()}
              </div>
            </div>
            <SignOutButton />
          </div>
        </div>
      </header>
    </>
  );
}

function ShieldMark() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 16 16"
      fill="none"
      aria-hidden="true"
      className="text-foreground"
    >
      <path
        d="M8 1.5 2.5 3.6v4.2c0 3.1 2.3 5.6 5.5 6.7 3.2-1.1 5.5-3.6 5.5-6.7V3.6L8 1.5Z"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinejoin="round"
      />
      <circle cx="8" cy="7.2" r="1.5" stroke="currentColor" strokeWidth="1.3" />
      <path
        d="M8 8.7v2.1"
        stroke="currentColor"
        strokeWidth="1.3"
        strokeLinecap="round"
      />
    </svg>
  );
}
