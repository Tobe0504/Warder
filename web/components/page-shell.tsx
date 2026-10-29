import { Crumb, CrumbSeparator, PageHeader } from "@/components/page-header";
import { cn } from "@/lib/utils";

export { Crumb, CrumbSeparator };

/**
 * The measure every page in the application is set to, public and dashboard
 * alike.
 *
 * One value, used by the header bar and the content alike. The two used
 * different rhythms before: the bar ran edge to edge while the content was
 * centred under a cap, so the breadcrumb sat far to the left of the title it
 * belonged to and nothing on the page shared a left edge.
 *
 * 72rem rather than the old 80rem, because the content is tables of six short
 * columns. Past this width the row stops being scannable and becomes a line to
 * track across.
 *
 * Exported because the site header, the documentation and the landing page
 * have to agree with it. They did not: the headers ran to 80rem while the
 * content under them stopped at 72, so every page had a header indented
 * differently from the thing it belonged to.
 */
export const MEASURE = "max-w-7xl";

/**
 * The standard dashboard page.
 *
 * Three bands, in order: a thin bar carrying the path, a page header carrying
 * the name and the action that belongs to it, then the content. The bar is
 * full bleed so it reads as chrome; everything with meaning in it is aligned
 * to one column.
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
  /** Opts out of the measure, for full-bleed tables. */
  wide?: boolean;
}) {
  const measure = wide ? "max-w-7xl" : MEASURE;

  return (
    <>
      <PageHeader breadcrumb={breadcrumb} actions={actions} measure={measure} />
      <main id="main" className="flex-1">
        <div className={cn("mx-auto w-full px-4 py-5 sm:px-6", measure)}>
          {children}
        </div>
      </main>
    </>
  );
}

/**
 * A page's header: what you are looking at, and the action that belongs to it.
 *
 * Given real weight on purpose. The previous version dropped the name entirely
 * to avoid repeating the breadcrumb, which left every page opening on a grey
 * sentence and nothing to anchor the eye. The breadcrumb is the path and this
 * is the destination; a page that names itself is worth the one line it costs.
 *
 * The rule underneath separates the header band from the content, so a table
 * starts against an edge rather than floating in the middle of the page.
 */
export function PageTitle({
  title,
  description,
  actions,
  children,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  /** Tabs or filters that belong to the header band rather than the content. */
  children?: React.ReactNode;
}) {
  return (
    <div className={cn("mb-5 border-b pb-4", children && "pb-0")}>
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <h1 className="text-title font-semibold tracking-tight">{title}</h1>
          {description && (
            <p className="mt-1 max-w-[72ch] text-meta leading-relaxed text-muted-foreground">
              {description}
            </p>
          )}
        </div>
        {actions && (
          <div className="flex shrink-0 items-center gap-2">{actions}</div>
        )}
      </div>

      {children && <div className="mt-4">{children}</div>}
    </div>
  );
}
