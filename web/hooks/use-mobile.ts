"use client";

import * as React from "react";

const MOBILE_BREAKPOINT = 768;

/**
 * Reports whether the viewport is narrow enough that the sidebar should become
 * an overlay rather than a column.
 *
 * Starts undefined and resolves after mount, so the first server-rendered pass
 * does not guess. Guessing produces a layout that visibly jumps on load.
 */
export function useIsMobile() {
  const [isMobile, setIsMobile] = React.useState<boolean | undefined>(undefined);

  React.useEffect(() => {
    const query = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`);
    const onChange = () => setIsMobile(window.innerWidth < MOBILE_BREAKPOINT);
    query.addEventListener("change", onChange);
    setIsMobile(window.innerWidth < MOBILE_BREAKPOINT);
    return () => query.removeEventListener("change", onChange);
  }, []);

  return !!isMobile;
}
