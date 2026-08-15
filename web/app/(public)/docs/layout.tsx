import { ChevronDown } from "lucide-react";

import { DocsNav } from "@/components/docs/docs-nav";
import { DOC_SECTIONS } from "@/lib/docs";

export const dynamic = "force-static";

export default function DocsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  // Only what the sidebar needs crosses to the client: titles and slugs.
  const sections = DOC_SECTIONS.map((section) => ({
    title: section.title,
    docs: section.docs.map((doc) => ({ slug: doc.slug, title: doc.title })),
  }));

  return (
    <div className="mx-auto max-w-7xl px-4 sm:px-6">
      {/*
        On a narrow screen the navigation is a disclosure above the content
        rather than a drawer. It needs no JavaScript, it cannot get stuck open
        over the text, and the page it belongs to is the thing someone came to
        read.
      */}
      <details className="group border-b py-3 md:hidden">
        <summary className="flex cursor-pointer list-none items-center gap-1.5 text-meta font-medium">
          All documentation
          <ChevronDown className="size-3.5 text-muted-foreground transition-transform duration-150 group-open:rotate-180" />
        </summary>
        <div className="pt-4">
          <DocsNav sections={sections} />
        </div>
      </details>

      <div className="md:grid md:grid-cols-[13rem_minmax(0,1fr)] md:gap-12">
        <aside className="hidden md:sticky md:top-14 md:block md:h-[calc(100dvh-3.5rem)] md:overflow-y-auto md:py-10">
          <DocsNav sections={sections} />
        </aside>

        <div className="min-w-0 py-10">{children}</div>
      </div>
    </div>
  );
}
