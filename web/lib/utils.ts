import { clsx, type ClassValue } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

/**
 * Class merging, taught about this project's type scale.
 *
 * The scale is named `text-title`, `text-body`, `text-meta` and so on, which
 * puts it in the same `text-*` prefix as every text colour. tailwind-merge
 * resolves conflicts by class group, and it only knows its own built-in font
 * sizes, so an unrecognised `text-meta` is filed as a text *colour*, and
 * merging a sized button drops the real colour as a duplicate.
 *
 * That failed silently and looked like a theming bug: a primary button rendered
 * white text on a white background, because `text-primary-foreground` had been
 * removed from the class list before it ever reached the DOM.
 *
 * Registering the scale as font sizes puts each utility in the right group, so
 * a size and a colour can coexist. Anything added to the scale in globals.css
 * has to be added here too.
 */
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      "font-size": [{ text: ["title", "heading", "body", "meta", "micro"] }],
    },
  },
});

/** Merges Tailwind classes, letting later classes win over earlier ones. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** Formats a timestamp for display, or a dash when absent. */
export function formatDate(value: string | null | undefined): string {
  if (!value) return ", ";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return ", ";
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

/** Describes how long until a timestamp, for expiry columns. */
export function formatRelative(value: string | null | undefined): string {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return ", ";

  const seconds = Math.round((date.getTime() - Date.now()) / 1000);
  const past = seconds < 0;
  const magnitude = Math.abs(seconds);

  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ["day", 86400],
    ["hour", 3600],
    ["minute", 60],
  ];

  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  for (const [unit, size] of units) {
    if (magnitude >= size) {
      return formatter.format(past ? -Math.floor(magnitude / size) : Math.floor(magnitude / size), unit);
    }
  }
  return past ? "just now" : "in under a minute";
}
