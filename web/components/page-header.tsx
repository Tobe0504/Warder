import { SidebarTrigger } from "@/components/ui/sidebar";

/**
 * The bar across the top of every dashboard page.
 *
 * Deliberately thin — 48px, like Vercel's — because the content below it is
 * dense tables and every pixel spent on chrome is a row someone cannot see.
 * It holds the sidebar toggle, a breadcrumb, and whatever action belongs to the
 * page.
 */
export function PageHeader({
  breadcrumb,
  actions,
}: {
  breadcrumb: React.ReactNode;
  actions?: React.ReactNode;
}) {
  return (
    <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-2 border-b bg-background/90 px-4 backdrop-blur">
      <SidebarTrigger className="-ml-1" />
      <div className="mx-1 h-4 w-px bg-border" />
      <div className="flex min-w-0 flex-1 items-center gap-2 text-body">{breadcrumb}</div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
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
