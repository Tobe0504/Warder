/**
 * Timestamp formatting.
 *
 * Kept out of lib/utils.ts on purpose: that file is owned by the shadcn CLI,
 * and every `shadcn add` (which is how beUI components arrive) overwrites it
 * with its own two-line version. Anything living there disappears without
 * warning.
 */

/** Formats a timestamp for display, or a dash when absent. */
export function formatDate(value: string | null | undefined): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

/** Describes how long until a timestamp, for expiry columns. */
export function formatRelative(value: string | null | undefined): string {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";

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
      const count = Math.floor(magnitude / size);
      return formatter.format(past ? -count : count, unit);
    }
  }
  return past ? "just now" : "in under a minute";
}
