"use client";

import { useEffect, useState } from "react";
import { Monitor, Moon, Sun } from "lucide-react";

import { cn } from "@/lib/utils";

type Theme = "light" | "dark" | "system";

const COOKIE = "warder_theme";

const OPTIONS: { value: Theme; label: string; icon: typeof Sun }[] = [
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
  { value: "system", label: "Device", icon: Monitor },
];

/**
 * Light, dark, or follow the device.
 *
 * The choice is stored in a cookie rather than localStorage. Partly because the
 * boundary check forbids browser storage outright, a rule worth keeping blunt,
 * since the cost of arguing about exceptions is how storage rules erode, and
 * partly because a cookie is the one that could later be read on the server if
 * this ever wanted to render the right theme on the first paint.
 *
 * "Device" is the default and stores nothing. It is also the only state with no
 * flash: the pages here are static HTML, so an explicit choice cannot be known
 * until this component mounts, and someone who picks dark on a light machine
 * will see one light frame first. Fixing that needs a blocking inline script,
 * which needs the CSP nonce, which would make every public page dynamic, a
 * poor trade for one frame.
 */
export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>("system");

  useEffect(() => {
    // The page arrives as static HTML with no attribute on it, so the stored
    // choice has to be applied here as well as recorded in state: otherwise
    // the toggle would show "Dark" while the page rendered light.
    const stored = readTheme();
    setTheme(stored);
    apply(stored);
  }, []);

  function choose(next: Theme) {
    setTheme(next);
    apply(next);
    // A year, and Lax rather than Strict: this is a display preference, and
    // arriving from an external link should not reset how the page looks.
    document.cookie = `${COOKIE}=${next}; path=/; max-age=31536000; samesite=lax`;
  }

  return (
    <div
      role="group"
      aria-label="Colour theme"
      className="inline-flex items-center gap-0.5 rounded-md border p-0.5"
    >
      {OPTIONS.map(({ value, label, icon: Icon }) => {
        const active = theme === value;
        return (
          <button
            key={value}
            type="button"
            onClick={() => choose(value)}
            aria-pressed={active}
            title={label}
            className={cn(
              "rounded-sm p-1 transition-colors",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              active
                ? "bg-muted text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <Icon className="size-3.5" />
            <span className="sr-only">{label}</span>
          </button>
        );
      })}
    </div>
  );
}

function readTheme(): Theme {
  const match = document.cookie.match(/(?:^|;\s*)warder_theme=(light|dark|system)/);
  return (match?.[1] as Theme) ?? "system";
}

/**
 * Stamps the choice on the document.
 *
 * `system` removes the attribute rather than setting it, which hands control
 * back to the prefers-color-scheme rules in globals.css, including live
 * updates when the device switches at sunset.
 */
function apply(theme: Theme) {
  const root = document.documentElement;
  if (theme === "system") {
    delete root.dataset.theme;
  } else {
    root.dataset.theme = theme;
  }
}
