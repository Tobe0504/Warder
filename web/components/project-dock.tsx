"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { KeyRound, KeySquare, ScrollText, ShieldCheck } from "lucide-react";

import { Dock, DockItem } from "@/components/motion/dock";
import { Tooltip } from "@/components/motion/tooltip";

const SECTIONS = [
  { label: "Secrets", segment: "", icon: KeyRound, hint: "What exists here" },
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
 * The project's own navigation, as a floating dock.
 *
 * This used to be a second column beside the organization sidebar, which put
 * two nav rails on screen at once and left the secrets table sharing the width
 * with both. A dock costs no horizontal space at all, which is what a table of
 * six-fact rows actually needs.
 *
 * Icon-only, so each item carries a tooltip: nothing here is guessable from a
 * key or a shield alone. The label is the name and the hint is what the section
 * answers, because "Access" and "Tokens" are both plausible homes for a
 * credential and the distinction is the whole point.
 */
export function ProjectDock({ projectId }: { projectId: string }) {
  const pathname = usePathname();
  const base = `/projects/${projectId}`;

  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-0 z-40 flex justify-center pb-[max(1rem,env(safe-area-inset-bottom))]">
      <nav aria-label="Project sections" className="pointer-events-auto">
        <Dock magnify className="gap-1 px-2 py-1.5">
          {SECTIONS.map((section) => {
            const href = `${base}${section.segment}`;
            // Secrets is the project's index, and an environment lives under
            // it, so both keep that item lit.
            const active =
              section.segment === ""
                ? pathname === base || pathname.includes("/environments/")
                : pathname.startsWith(href);
            const Icon = section.icon;

            return (
              <Tooltip
                key={section.label}
                side="top"
                content={
                  <span className="flex items-baseline gap-1.5">
                    <span className="font-medium">{section.label}</span>
                    <span className="text-muted-foreground">
                      {section.hint}
                    </span>
                  </span>
                }
              >
                {/*
                  The link is the interactive element, not the dock item: an
                  anchor can be opened in a new tab and reads as a destination,
                  which a button calling router.push cannot.
                */}
                <DockItem active={active}>
                  <Link
                    href={href}
                    aria-label={section.label}
                    aria-current={active ? "page" : undefined}
                    className="flex size-full items-center justify-center rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
                  >
                    <Icon
                      className={
                        active
                          ? "size-[18px] text-foreground"
                          : "size-[18px] text-muted-foreground transition-colors hover:text-foreground"
                      }
                    />
                  </Link>
                </DockItem>
              </Tooltip>
            );
          })}
        </Dock>
      </nav>
    </div>
  );
}
