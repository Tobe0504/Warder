"use client";

import { useEffect, useState } from "react";

import { formatDate, formatRelative } from "@/lib/utils";

/**
 * A timestamp rendered as "3 minutes ago", kept honest over time.
 *
 * Relative times are the one thing in this interface that genuinely cannot
 * match between server and client: the server renders at one instant and the
 * browser hydrates at another, so "just now" and "1 minute ago" disagree and
 * React reports a hydration mismatch. Every page in the dashboard shows several
 * of these, so the console filled with errors that were not really errors —
 * which is worse than useless, because it hides the ones that are.
 *
 * `suppressHydrationWarning` is the sanctioned answer for exactly this case: it
 * silences the comparison for this element's text and nothing else. The
 * interval then keeps the value current, so a dashboard left open does not sit
 * there insisting a token was used "just now" an hour later.
 *
 * The absolute time is always in the title attribute, because "2 hours ago" is
 * the wrong precision the moment anyone is actually investigating something.
 */
export function RelativeTime({
  value,
  prefix,
  fallback = "Never",
  className,
}: {
  value: string | null | undefined;
  /** Rendered before the time, e.g. "Created". */
  prefix?: string;
  /** Shown when there is no timestamp at all. */
  fallback?: string;
  className?: string;
}) {
  const [text, setText] = useState(() => (value ? formatRelative(value) : fallback));

  useEffect(() => {
    if (!value) {
      setText(fallback);
      return;
    }

    const update = () => setText(formatRelative(value));
    update();

    // A minute is finer than any of these values needs, and coarse enough that
    // a page left open overnight costs nothing.
    const timer = setInterval(update, 60_000);
    return () => clearInterval(timer);
  }, [value, fallback]);

  return (
    <span className={className} title={value ? formatDate(value) : undefined} suppressHydrationWarning>
      {prefix ? `${prefix} ` : ""}
      {text}
    </span>
  );
}
