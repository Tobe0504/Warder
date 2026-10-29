import { PanelLeft } from "lucide-react";

import { AnimatedSidebarTrigger } from "@/components/motion/animated-sidebar";
import { cn } from "@/lib/utils";

/**
 * The bar across the top of every dashboard page.
 *
 * Deliberately thin: 48px, like Vercel's, because the content below it is
 * dense tables and every pixel spent on chrome is a row someone cannot see.
 * It holds the sidebar toggle, a breadcrumb, and whatever action belongs to the
 * page.
 */
export function PageHeader({
  breadcrumb,
  actions,
  measure,
}: {
  breadcrumb: React.ReactNode;
  actions?: React.ReactNode;
  /** The page's measure, so the bar's contents share its left edge. */
  measure?: string;
}) {
  return (
    <header className="sticky top-0 z-20 h-11 shrink-0 border-b bg-background/90 backdrop-blur">
      {/*
        The bar spans the window; what is in it does not. Aligning the
        breadcrumb to the same column as the title below is what makes the two
        read as one page rather than as a bar with a page under it.
      */}
      <div
        className={cn(
          "mx-auto flex h-full w-full items-center gap-2 px-4 sm:px-6",
          measure,
        )}
      >
        <AnimatedSidebarTrigger className="-ml-1.5 size-7 shrink-0 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground">
          <PanelLeft aria-hidden="true" className="size-4" />
        </AnimatedSidebarTrigger>
        <div className="flex min-w-0 flex-1 items-center gap-1.5 text-meta text-muted-foreground">
          {breadcrumb}
        </div>
        {actions && (
          <div className="flex shrink-0 items-center gap-2">{actions}</div>
        )}
      </div>
    </header>
  );
}

/** One step in the breadcrumb. */
export function Crumb({
  children,
  muted,
  mono,
}: {
  children: React.ReactNode;
  muted?: boolean;
  mono?: boolean;
}) {
  return (
    <span
      className={[
        "truncate",
        muted ? "text-muted-foreground" : "text-foreground",
        mono ? "font-mono text-meta" : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {children}
    </span>
  );
}

/** The separator between breadcrumb steps. */
export function CrumbSeparator() {
  return (
    <span aria-hidden="true" className="select-none text-border">
      /
    </span>
  );
}
