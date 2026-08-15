import Link from "next/link";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { ALL_DOCS, slugifyHeading } from "@/lib/docs";
import { cn } from "@/lib/utils";

/**
 * Renders a documentation page.
 *
 * `react-markdown` produces React elements rather than an HTML string, so
 * nothing here goes through `dangerouslySetInnerHTML`, which the boundary
 * check forbids outright, and which would be a poor look on a security
 * product's own website.
 *
 * Every element is mapped to the same type scale and spacing the dashboard
 * uses, so the documentation reads as part of the product rather than as a
 * README someone pointed a renderer at.
 */
export function Markdown({ body, file }: { body: string; file: string }) {
  const components = markdownComponents(file);

  /*
   * No width cap on the container.
   *
   * The measure is capped on the elements that need it, `prose-note` puts a
   * 68ch limit on every paragraph and list, rather than on the wrapper. A cap
   * here would apply to tables and code blocks too, and the widest tables in
   * these documents (the threat model's asset inventory, for one) then get
   * squeezed into three cramped columns while the column beside them sits
   * empty.
   */
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
      {body}
    </ReactMarkdown>
  );
}

/**
 * Maps a link in a markdown file to a route on this site.
 *
 * The documents cross-reference each other with ordinary relative paths, which
 * is what keeps them readable in a terminal and in a pull request. Those paths
 * mean nothing to a browser here, so they are resolved against the linking
 * document and looked up in the manifest.
 */
const FILE_TO_SLUG = new Map(ALL_DOCS.map((doc) => [doc.file, doc.slug]));

function resolveDocHref(href: string, fromFile: string): string | null {
  const [path = "", hash] = href.split("#");
  const directory = fromFile.includes("/") ? fromFile.replace(/\/[^/]+$/, "") : "";

  // Resolve "../foo.md" and "bar.md" against the linking document's directory.
  const segments = (directory ? `${directory}/${path}` : path).split("/");
  const stack: string[] = [];
  for (const segment of segments) {
    if (segment === "." || segment === "") continue;
    if (segment === "..") stack.pop();
    else stack.push(segment);
  }

  const slug = FILE_TO_SLUG.get(stack.join("/"));
  if (!slug) return null;
  return `/docs/${slug}${hash ? `#${hash}` : ""}`;
}

function markdownComponents(file: string): Components {
  return {
    h1: ({ children }) => (
      <h2 {...headingProps(children, "text-title font-semibold mt-10 mb-3")}>{children}</h2>
    ),

    h2: ({ children }) => (
      <h2
        {...headingProps(
          children,
          "text-title font-semibold mt-12 mb-3 pt-6 border-t first:mt-0 first:border-t-0 first:pt-0",
        )}
      >
        {children}
      </h2>
    ),

    h3: ({ children }) => (
      <h3 {...headingProps(children, "text-heading font-semibold mt-8 mb-2")}>{children}</h3>
    ),

    h4: ({ children }) => (
      <h4 {...headingProps(children, "text-body font-medium mt-6 mb-2 text-muted-foreground")}>
        {children}
      </h4>
    ),

    p: ({ children }) => <p className="prose-doc my-4 text-muted-foreground">{children}</p>,

    // Emphasis carries the load-bearing sentences in these documents, so it
    // steps up to full-contrast foreground rather than staying muted.
    strong: ({ children }) => (
      <strong className="font-semibold text-foreground">{children}</strong>
    ),

    a: ({ href, children }) => {
      if (!href) return <>{children}</>;

      if (href.startsWith("#")) {
        return (
          <a href={href} className="underline underline-offset-2 decoration-border hover:decoration-foreground">
            {children}
          </a>
        );
      }

      if (/^https?:\/\//.test(href)) {
        return (
          <a
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            className="underline underline-offset-2 decoration-border hover:decoration-foreground"
          >
            {children}
          </a>
        );
      }

      const internal = resolveDocHref(href, file);
      if (internal) {
        return (
          <Link
            href={internal}
            className="font-medium text-foreground underline underline-offset-2 decoration-border hover:decoration-foreground"
          >
            {children}
          </Link>
        );
      }

      // A path into the source tree, `../internal/cli/run.go` and the like.
      // There is no page for it, and a dead link is worse than none, so it
      // renders as the file path it is.
      return <code className="rounded bg-muted px-1 py-0.5 font-mono text-meta">{children}</code>;
    },

    ul: ({ children }) => (
      <ul className="my-4 space-y-2 prose-doc text-muted-foreground [&_ul]:mt-2 [&_ul]:ml-5">
        {children}
      </ul>
    ),

    ol: ({ children }) => (
      <ol className="my-4 ml-5 list-decimal space-y-2 prose-doc text-muted-foreground marker:text-muted-foreground">
        {children}
      </ol>
    ),

    li: ({ children }) => (
      <li className="ml-1 pl-1 [ul>&]:list-disc [ul>&]:ml-4 marker:text-border">{children}</li>
    ),

    blockquote: ({ children }) => (
      <blockquote className="my-5 rounded-r-md border-l-2 border-can-reveal bg-can-reveal-surface/40 py-1 pl-4 pr-3 [&_p]:text-foreground">
        {children}
      </blockquote>
    ),

    pre: ({ children }) => (
      <pre className="my-4 overflow-x-auto rounded-lg border bg-muted/60 p-3">{children}</pre>
    ),

    code: ({ className, children }) => {
      const isBlock = /language-/.test(className ?? "");
      if (isBlock) {
        return <code className="font-mono text-meta leading-relaxed">{children}</code>;
      }
      return (
        <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.9em] text-foreground">
          {children}
        </code>
      );
    },

    // Tables are wide by nature and must scroll inside their own container
    // rather than pushing the page sideways.
    table: ({ children }) => (
      <div className="my-5 overflow-x-auto rounded-lg border">
        <table className="w-full border-collapse text-left">{children}</table>
      </div>
    ),

    thead: ({ children }) => <thead className="border-b bg-muted/50">{children}</thead>,

    th: ({ children }) => (
      <th className="whitespace-nowrap px-3 py-2 text-body font-medium">{children}</th>
    ),

    tr: ({ children }) => <tr className="border-b last:border-b-0">{children}</tr>,

    td: ({ children }) => (
      <td className="px-3 py-2 align-top text-body text-muted-foreground [&>code]:whitespace-nowrap">
        {children}
      </td>
    ),

    hr: () => <hr className="my-10 border-t" />,
  };
}

/**
 * Gives a heading a stable anchor.
 *
 * Several of these documents open with their own hand-written contents list
 * pointing at `#some-heading`. Those links only work if the ids generated here
 * match, which is why both sides go through the same slug function.
 *
 * `scroll-mt` keeps the target clear of the sticky site header when someone
 * follows one of those links.
 */
function headingProps(children: React.ReactNode, className: string) {
  return {
    id: slugifyHeading(textOf(children)),
    className: cn("scroll-mt-24 text-foreground", className),
  };
}

/** Flattens a heading's rendered children back to plain text for slugging. */
function textOf(node: React.ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (node && typeof node === "object" && "props" in node) {
    const props = (node as { props?: { children?: React.ReactNode } }).props;
    return textOf(props?.children);
  }
  return "";
}
