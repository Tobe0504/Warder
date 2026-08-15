import { cn } from "@/lib/utils";

/**
 * A placeholder block shown while content loads.
 *
 * The pulse is suppressed under prefers-reduced-motion by the global rule in
 * globals.css, leaving a static block, which still communicates "content
 * goes here" without moving.
 */
export function Skeleton({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("animate-pulse rounded-md bg-muted", className)} {...props} />;
}
