import Link from "next/link";

import { Mark, Wordmark } from "@/components/logo";
import { ThemeToggle } from "@/components/theme-toggle";
import UserAvatar from "@/components/user-avatar";

/**
 * The public site's header.
 *
 * A server component that reads nothing per-request. The account control is a
 * client component that asks /api/auth/session after the page loads, which is
 * what lets the landing page and the documentation stay static HTML.
 *
 * It deliberately does not gate access. Someone who is not signed in came here
 * to read the docs, and redirecting them to sign-in would make a public site
 * private.
 */
export function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 bg-background/80 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-7xl items-center gap-6 px-4 sm:px-6">
        <Link href="/" className="rounded-md" aria-label="Warder home">
          <Wordmark height={19} priority />
        </Link>

        <UserAvatar />
      </div>
    </header>
  );
}

export function SiteFooter() {
  return (
    <footer className="border-t">
      <div className="mx-auto flex max-w-7xl flex-col gap-3 px-4 py-6 text-meta text-muted-foreground sm:flex-row sm:items-center sm:px-6">
        <p className="flex items-center gap-2">
          <Mark size={14} />
          Warder: Secret access without unnecessary secret visibility.
        </p>
        <nav className="flex flex-wrap items-center gap-x-4 gap-y-1 sm:ml-auto">
          <Link
            href="/docs/using-warder"
            className="rounded transition-colors hover:text-foreground"
          >
            Get started
          </Link>
          <Link
            href="/docs/security/threat-model"
            className="rounded transition-colors hover:text-foreground"
          >
            Threat model
          </Link>
          <Link
            href="/docs/security/limitations"
            className="rounded transition-colors hover:text-foreground"
          >
            Limitations
          </Link>

          <ThemeToggle />
        </nav>
      </div>
    </footer>
  );
}
