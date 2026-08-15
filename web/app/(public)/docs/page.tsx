import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { DOC_SECTIONS } from "@/lib/docs";

export const metadata: Metadata = {
  title: "Documentation",
  description:
    "How to give an application the credentials it needs without handing them to everyone who builds it.",
};

export const dynamic = "force-static";

export default function DocsIndexPage() {
  return (
    <div className="max-w-[68ch]">
      <h1 className="text-title font-medium tracking-tight sm:text-[1.75rem]">Documentation</h1>
      <p className="mt-3 prose-note text-muted-foreground">
        Everything here is generated from the markdown that ships with the repository, so a page
        cannot drift from the version of the code it describes. If you are integrating an existing
        application, start with{" "}
        <Link
          href="/docs/using-warder"
          className="font-medium text-foreground underline underline-offset-2 decoration-border hover:decoration-foreground"
        >
          Using Warder in your application
        </Link>
        .
      </p>

      <div className="mt-12 space-y-12">
        {DOC_SECTIONS.map((section) => (
          <section key={section.title}>
            <h2 className="text-heading font-semibold">{section.title}</h2>
            <p className="mt-1.5 prose-note text-muted-foreground">{section.blurb}</p>

            <ul className="mt-4 space-y-2">
              {section.docs.map((doc) => (
                <li key={doc.slug}>
                  <Link
                    href={`/docs/${doc.slug}`}
                    className="group flex items-start gap-3 rounded-xl border bg-card p-4 transition-colors hover:bg-muted/40"
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block text-body font-medium">{doc.title}</span>
                      <span className="mt-1 block text-meta leading-relaxed text-muted-foreground">
                        {doc.summary}
                      </span>
                    </span>
                    <ArrowRight className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform duration-150 ease-out group-hover:translate-x-0.5" />
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
