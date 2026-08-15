import { Skeleton } from "@/components/ui/skeleton";

/**
 * Shown while a dashboard page's data is being fetched.
 *
 * The shape matches what is about to appear — a heading, then rows — so the
 * page does not jump when the content arrives. A spinner would say "something
 * is happening"; this says "a table is coming".
 */
export default function DashboardLoading() {
  return (
    <div className="space-y-4" aria-busy="true" aria-live="polite">
      <span className="sr-only">Loading</span>

      <div className="space-y-2">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-3 w-64" />
      </div>

      <div className="space-y-2 rounded-lg border p-3">
        {[0, 1, 2, 3].map((row) => (
          <div key={row} className="flex items-center gap-3">
            <Skeleton className="h-3 w-40" />
            <Skeleton className="h-3 w-20" />
            <Skeleton className="ml-auto h-3 w-24" />
          </div>
        ))}
      </div>
    </div>
  );
}
