import { Boxes } from "lucide-react";

import { cn } from "@/lib/utils";

/**
 * Initials on a flat surface. No external image request, nothing to track.
 *
 * Lives here rather than inside the dashboard sidebar so that the public
 * header can use it without pulling the entire signed-in navigation — and its
 * client-side dependencies — into the landing page's bundle.
 */
export function Avatar({ name, className }: { name: string; className?: string }) {
  const initials = name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");

  return (
    <div
      aria-hidden="true"
      className={cn(
        "flex aspect-square size-7 shrink-0 items-center justify-center rounded-full border bg-muted text-meta font-medium",
        className,
      )}
    >
      {initials || <Boxes className="size-3.5" />}
    </div>
  );
}
