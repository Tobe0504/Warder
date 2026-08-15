import { cn } from "@/lib/utils";

/**
 * The dense row list this dashboard is built from.
 *
 * Modelled on Vercel's deployments table, and a list rather than a `<table>`
 * on purpose. These rows are records with a handful of labelled facts, not a
 * grid people scan down a single column of, and a list lets each row reflow
 * on a narrow screen instead of forcing a horizontal scrollbar across the page.
 *
 * Where the data really is tabular, audit events, version history, the table
 * primitives are still the right tool.
 */

export function DataList({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("overflow-hidden rounded-lg border bg-card", className)}
      {...props}
    />
  );
}

export function DataListRow({
  className,
  interactive,
  ...props
}: React.HTMLAttributes<HTMLDivElement> & { interactive?: boolean }) {
  return (
    <div
      className={cn(
        "flex items-center gap-8 border-b px-3.5 py-2 last:border-b-0",
        interactive && "transition-colors hover:bg-muted/40",
        className,
      )}
      {...props}
    />
  );
}

/** The column headings above a DataList. */
export function DataListHeader({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 border-b bg-muted/30 px-3.5 py-2",
        "text-micro font-medium uppercase text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}

/**
 * A status dot with its label, as Vercel marks deployment state.
 *
 * The dot carries colour and the word carries meaning, so the state is
 * readable without relying on colour alone.
 */
const STATUS_TONE = {
  ready: "bg-can-use",
  pending: "bg-can-reveal",
  error: "bg-destructive",
  neutral: "bg-muted-foreground/50",
} as const;

export function StatusDot({
  tone = "neutral",
  label,
  pulse,
}: {
  tone?: keyof typeof STATUS_TONE;
  label: string;
  pulse?: boolean;
}) {
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-body">
      <span className="relative flex size-1.5 shrink-0">
        {pulse && (
          <span
            className={cn(
              "absolute inline-flex size-full animate-ping rounded-full opacity-60",
              STATUS_TONE[tone],
            )}
          />
        )}
        <span
          className={cn(
            "relative inline-flex size-1.5 rounded-full",
            STATUS_TONE[tone],
          )}
        />
      </span>
      <span className="capitalize">{label}</span>
    </span>
  );
}

/**
 * A monospace identifier, a commit SHA in Vercel's table, a secret key or
 * token prefix here.
 */
export function MonoTag({
  children,
  icon,
  className,
}: {
  children: React.ReactNode;
  icon?: React.ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 whitespace-nowrap font-mono text-meta text-muted-foreground",
        className,
      )}
    >
      {icon}
      {children}
    </span>
  );
}

/** The right-hand metadata column: relative time, then who. */
export function RowMeta({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "ml-auto flex shrink-0 items-center gap-4 text-meta tabular text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}

/** An empty state that names the next action rather than just the absence. */
export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-3 px-6 py-16 text-center">
      {icon && <div className="text-muted-foreground">{icon}</div>}
      <p className="text-heading font-medium">{title}</p>
      {description && (
        <p className="prose-note text-center text-muted-foreground">
          {description}
        </p>
      )}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
