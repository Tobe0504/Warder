import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft, ArrowRight } from "lucide-react";

import { Markdown } from "@/components/docs/markdown";
import { ALL_DOCS, docNeighbours, findDoc, loadDoc } from "@/lib/docs";

type Params = { params: Promise<{ slug: string[] }> };

export const dynamic = "force-static";

/**
 * Enumerates every page at build time.
 *
 * With this in place the markdown is read during `next build` and the output is
 * static HTML, so the `docs/` directory is not a run-time dependency of the
 * deployed application.
 */
export function generateStaticParams() {
  return ALL_DOCS.map((doc) => ({ slug: doc.slug.split("/") }));
}

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const { slug } = await params;
  const meta = findDoc(slug.join("/"));
  if (!meta) return {};

  // The root layout's title template appends the product name.
  return { title: meta.title, description: meta.summary };
}

export default async function DocPage({ params }: Params) {
  const { slug } = await params;
  const doc = await loadDoc(slug.join("/"));
  if (!doc) notFound();

  const { previous, next } = docNeighbours(doc.meta.slug);

  /*
   * Long documents list only their top-level headings in the rail.
   *
   * The threat model has over thirty headings; including every third-level one
   * produces a rail that fills the viewport and needs scrolling of its own,
   * which is a worse way to find a section than the outline it replaced.
   */
  const rail = doc.headings.length > 20
    ? doc.headings.filter((heading) => heading.level === 2)
    : doc.headings;

  return (
    <div className="xl:grid xl:grid-cols-[minmax(0,1fr)_11rem] xl:gap-10">
      <article className="min-w-0">
        <header className="mb-10">
          <h1 className="text-title font-medium tracking-tight sm:text-[1.75rem]">
            {doc.meta.title}
          </h1>
          <p className="mt-2.5 max-w-[62ch] prose-doc text-muted-foreground">
            {doc.meta.summary}
          </p>
        </header>

        <Markdown body={doc.body} file={doc.meta.file} />

        <nav className="mt-16 grid gap-2 border-t pt-6 sm:grid-cols-2">
          {previous ? (
            <Link
              href={`/docs/${previous.slug}`}
              className="group rounded-xl border p-3 transition-colors hover:bg-muted/40"
            >
              <span className="flex items-center gap-1 text-meta text-muted-foreground">
                <ArrowLeft className="size-3 transition-transform duration-150 ease-out group-hover:-translate-x-0.5" />
                Previous
              </span>
              <span className="mt-1 block text-meta font-medium">{previous.title}</span>
            </Link>
          ) : (
            <span />
          )}

          {next && (
            <Link
              href={`/docs/${next.slug}`}
              className="group rounded-xl border p-3 text-right transition-colors hover:bg-muted/40 sm:col-start-2"
            >
              <span className="flex items-center justify-end gap-1 text-meta text-muted-foreground">
                Next
                <ArrowRight className="size-3 transition-transform duration-150 ease-out group-hover:translate-x-0.5" />
              </span>
              <span className="mt-1 block text-meta font-medium">{next.title}</span>
            </Link>
          )}
        </nav>
      </article>

      {/*
        The contents rail. Only shown where there is genuine room for it:
        squeezing it in at narrower widths would cost the prose the measure it
        needs, and these documents are long enough that reading comfort matters
        more than a shortcut.
      */}
      {rail.length > 2 && (
        <aside className="hidden xl:sticky xl:top-20 xl:block xl:h-fit xl:max-h-[calc(100dvh-6rem)] xl:overflow-y-auto">
          <p className="mb-2 text-micro font-medium uppercase text-muted-foreground">
            On this page
          </p>
          <ul className="space-y-1.5 border-l">
            {rail.map((heading) => (
              <li key={`${heading.id}-${heading.text}`}>
                <a
                  href={`#${heading.id}`}
                  className={`-ml-px block border-l border-transparent pl-3 text-meta leading-snug text-muted-foreground transition-colors hover:border-foreground hover:text-foreground ${
                    heading.level === 3 ? "pl-6 text-muted-foreground/80" : ""
                  }`}
                >
                  {heading.text}
                </a>
              </li>
            ))}
          </ul>
        </aside>
      )}
    </div>
  );
}
