"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { KeyRound, KeySquare, ScrollText, ShieldCheck } from "lucide-react";

import { cn } from "@/lib/utils";

const SECTIONS = [
  {
    label: "Secrets",
    segment: "",
    icon: KeyRound,
    hint: "What exists here",
  },
  {
    label: "Access",
    segment: "/access",
    icon: ShieldCheck,
    hint: "Who can use and see it",
  },
  {
    label: "Tokens",
    segment: "/tokens",
    icon: KeySquare,
    hint: "Credentials runtimes hold",
  },
  {
    label: "Audit",
    segment: "/audit",
    icon: ScrollText,
    hint: "What has happened",
  },
] as const;

/**
 * The project's own navigation column.
 *
 * A second, nested nav rather than a tab strip. Tabs work while there are four
 * of them and stop working shortly after, labels start colliding, and the row
 * has nowhere to grow. A column has room for the sections this will accumulate
 * (environments, integrations, settings), keeps the active item on the same
 * axis as the main sidebar's, and leaves the horizontal space to the tables,
 * which is what actually needs it.
 *
 * On a narrow screen it lays back down as a scrolling row, because a
 * two-column layout at 375px is one column of nothing.
 */
export function ProjectNav({ projectId }: { projectId: string }) {
  const pathname = usePathname();
  const base = `/projects/${projectId}`;

  return (
    <aside className="shrink-0 md:w-40">
      <nav
        aria-label="Project sections"
        className={cn(
          "flex gap-2 overflow-x-auto pb-1 md:sticky md:top-[4.5rem]",
          "md:flex-col md:overflow-visible md:pb-0",
          // The scrolling row on mobile should not show a scrollbar over the
          // content beneath it.
          "[scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
        )}
      >
        <div className="hidden px-2 pb-1.5 text-meta font-medium text-muted-foreground md:block">
          Project
        </div>

        {SECTIONS.map((section) => {
          const href = `${base}${section.segment}`;
          const active =
            section.segment === ""
              ? pathname === base || pathname.includes("/environments/")
              : pathname.startsWith(href);

          const Icon = section.icon;

          return (
            <Link
              key={section.label}
              href={href}
              aria-current={active ? "page" : undefined}
              title={section.hint}
              className={cn(
                "flex shrink-0 items-center gap-2 rounded-md px-2.5 py-1.5 text-body transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                active
                  ? "bg-muted font-medium text-foreground"
                  : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
              )}
            >
              <Icon className="size-4 shrink-0" />
              <span>{section.label}</span>
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
