import Link from "next/link";

import { Wordmark } from "@/components/logo";

/**
 * The frame around sign-in and sign-up.
 *
 * Deliberately spare, in the proportions Vercel uses: a mark, a short heading,
 * a narrow card, and one line of alternate action underneath. Nothing else on
 * the page. An authentication screen that carries marketing copy or a
 * navigation bar gives a phishing clone more to imitate convincingly, and gives
 * the person signing in more to read before doing the one thing they came for.
 */
export function AuthShell({
  title,
  description,
  children,
  footer,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
}) {
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center px-4 py-12">
      <div className="w-full max-w-[360px]">
        <div className="mb-7 flex flex-col items-center text-center">
          {/*
            The one piece of branding on this screen. It is here so that
            somebody typing a password can see whose sign-in page they are on,
            which is the same reason the page carries nothing else.
          */}
          <Link href="/" className="rounded-md" aria-label="Warder">
            <Wordmark height={22} priority />
          </Link>

          <h1 className="mt-5 text-title font-medium tracking-tight">
            {title}
          </h1>
          {description && (
            <p className="mt-1.5 max-w-[300px] text-meta leading-relaxed text-muted-foreground">
              {description}
            </p>
          )}
        </div>

        <div className="rounded-xl border bg-card p-5">{children}</div>

        {footer && (
          <div className="mt-5 text-center text-meta text-muted-foreground">
            {footer}
          </div>
        )}
      </div>

      <p className="mt-10 max-w-[360px] text-center prose-note text-muted-foreground">
        Warder delivers credentials to the software that needs them, without
        handing them to everyone who builds it.
      </p>
    </main>
  );
}
