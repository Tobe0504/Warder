"use client";

import { ThemeToggle as MotionThemeToggle } from "@/components/motion/theme-toggle";
import { cn } from "@/lib/utils";

/**
 * Light and dark, from the footer.
 *
 * A thin wrapper over the beui component so the rest of the app imports one
 * name and the design system can be swapped underneath without touching call
 * sites. The variant gallery this replaced was the documentation's demo: four
 * toggles side by side with labels, which is right on a docs page and wrong in
 * a footer.
 *
 * `circle` starting bottom-up, because the control sits at the bottom of the
 * page and the reveal should look like it came from the thing that was
 * pressed.
 */
export function ThemeToggle({ className }: { className?: string }) {
  return (
    <MotionThemeToggle
      variant="circle-blur"
      start="bottom-up"
      className={cn(
        "rounded-md p-1 text-muted-foreground transition-colors hover:text-foreground",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className,
      )}
      iconClassName="size-3.5"
    />
  );
}
