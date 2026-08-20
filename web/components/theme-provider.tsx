"use client";

import { ThemeProvider as NextThemesProvider } from "next-themes";
import type { ComponentProps } from "react";

/**
 * The theme provider the beui components expect.
 *
 * `useTheme()` returns nothing without one mounted, which is why a toggle can
 * look wired up and do nothing at all: `resolvedTheme` is undefined, so the
 * button always thinks it is in light mode and `setTheme` writes somewhere the
 * page never reads.
 *
 * Two attributes, not one. The palette in globals.css keys off
 * `data-theme` — `:root[data-theme="dark"]`, and the `dark:` variant is
 * redefined to match — while shadcn and beui components key off a `.dark`
 * class. Setting both means the existing interface keeps working while
 * anything pulled in from the new design system also does, so components can
 * move over one at a time rather than in one commit.
 */
export function ThemeProvider({
  children,
  ...props
}: ComponentProps<typeof NextThemesProvider>) {
  return (
    <NextThemesProvider
      attribute={["class", "data-theme"]}
      defaultTheme="system"
      enableSystem
      {...props}
    >
      {children}
    </NextThemesProvider>
  );
}
