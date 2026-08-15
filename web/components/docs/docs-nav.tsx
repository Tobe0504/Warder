"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";

/**
 * The documentation sidebar.
 *
 * The section data is passed in from the server rather than imported, because
 * the manifest lives in a server-only module that reads the filesystem. What
 * arrives here is plain data — titles and slugs — and this component exists
 * only to mark the current page.
 */

type NavSection = {
  title: string;
  docs: { slug: string; title: string }[];
};

export function DocsNav({ sections }: { sections: NavSection[] }) {
  const pathname = usePathname();

  return (
    <nav aria-label="Documentation">
      <ul className="space-y-6">
        {sections.map((section) => (
          <li key={section.title}>
            <h2 className="mb-2 px-2 text-micro font-medium uppercase text-muted-foreground">
              {section.title}
            </h2>
            <ul className="space-y-0.5">
              {section.docs.map((doc) => {
                const href = `/docs/${doc.slug}`;
                const active = pathname === href;
                return (
                  <li key={doc.slug}>
                    <Link
                      href={href}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "block rounded-md px-2 py-1.5 text-meta leading-snug transition-colors",
                        active
                          ? "bg-muted font-medium text-foreground"
                          : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                      )}
                    >
                      {doc.title}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </li>
        ))}
      </ul>
    </nav>
  );
}
