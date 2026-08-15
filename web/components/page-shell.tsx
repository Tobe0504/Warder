import { Crumb, CrumbSeparator, PageHeader } from "@/components/page-header";
import { cn } from "@/lib/utils";

export { Crumb, CrumbSeparator };

/**
 * The standard dashboard page: a thin header bar, then content.
 *
 * Every page uses this so the header height, the content gutter, and the
 * maximum width are decided once. Tables are dense enough that inconsistent
 * padding between pages is immediately visible.
 */
export function PageShell({
  breadcrumb,
  actions,
  children,
  wide,
}: {
  breadcrumb: React.ReactNode;
  actions?: React.ReactNode;
  children: React.ReactNode;
  /** Opts out of the reading-width cap, for full-bleed tables. */
  wide?: boolean;
}) {
  return (
    <>
      <PageHeader breadcrumb={breadcrumb} actions={actions} />
      <main id="main" className="flex-1 px-4 py-6 sm:px-6">
        <div
          className={cn(
            "mx-auto w-full",
            wide ? "max-w-[1400px]" : "max-w-7xl",
          )}
        >
          {children}
        </div>
      </main>
    </>
  );
}

/**
 * A page's title block: name on the left, actions on the right.
 *
 * Separate from the header bar, which carries the breadcrumb. Vercel does the
 * same thing — the bar tells you where you are, this tells you what you are
 * looking at.
 */
export function PageTitle({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div className="min-w-0">
        <h1 className="text-title font-medium">{title}</h1>
        {description && (
          <p className="prose-note mt-1.5 text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      {actions && (
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      )}
    </div>
  );
}
