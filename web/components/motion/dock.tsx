"use client";
// beui.dev/components/motion/dock, extended with pointer magnification.

import {
  motion,
  useMotionValue,
  useReducedMotion,
  useSpring,
  useTransform,
  type MotionValue,
} from "motion/react";
import {
  createContext,
  useContext,
  useId,
  useMemo,
  useRef,
  type PointerEvent,
  type ReactNode,
} from "react";

import { SPRING_LAYOUT, SPRING_MOUSE } from "@/lib/ease";
import { useHoverCapable } from "@/lib/hooks/use-hover-capable";
import { cn } from "@/lib/utils";

type DockContextValue = {
  size: number;
  pillLayoutId: string;
  pointerX: MotionValue<number>;
  magnify: boolean;
  maxScale: number;
  reach: number;
};

const DockContext = createContext<DockContextValue | null>(null);

export interface DockProps {
  children: ReactNode;
  className?: string;
  /** Size of each item in px. */
  size?: number;
  /**
   * Grow items as the pointer nears them, as a desktop dock does. Ignored on
   * touch, where there is no pointer to be near anything.
   */
  magnify?: boolean;
  /** How large the item directly under the pointer becomes. */
  maxScale?: number;
  /** How far, in px, the pointer's influence reaches along the dock. */
  reach?: number;
}

export function Dock({
  children,
  size = 44,
  magnify = false,
  maxScale = 1.6,
  reach = 130,
  className,
}: DockProps) {
  const pillLayoutId = useId();
  const pointerX = useMotionValue(Number.POSITIVE_INFINITY);
  const reduce = useReducedMotion();
  const canHover = useHoverCapable();

  // Magnification is a pointer affordance and a decorative one. A finger has
  // no hover position to read, and someone who asked for less motion did not
  // ask for icons that resize under the cursor.
  const active = magnify && canHover && !reduce;

  const ctx = useMemo<DockContextValue>(
    () => ({ size, pillLayoutId, pointerX, magnify: active, maxScale, reach }),
    [size, pillLayoutId, pointerX, active, maxScale, reach],
  );

  return (
    <DockContext.Provider value={ctx}>
      <div
        onPointerMove={(event: PointerEvent<HTMLDivElement>) => {
          if (event.pointerType === "touch") return;
          pointerX.set(event.clientX);
        }}
        onPointerLeave={() => pointerX.set(Number.POSITIVE_INFINITY)}
        className={cn(
          "inline-flex h-auto items-end gap-1.5 rounded-2xl bg-card/80 px-2 py-1 shadow-2xl backdrop-blur-xl",
          className,
        )}
      >
        {children}
      </div>
    </DockContext.Provider>
  );
}

export interface DockItemProps {
  children: ReactNode;
  className?: string;
  /** When set, the item renders as a <button>. Omit when children carry their own link or button. */
  onClick?: () => void;
  active?: boolean;
  "aria-label"?: string;
}

export function DockItem({
  children,
  className,
  onClick,
  active,
  ...rest
}: DockItemProps) {
  const dock = useContext(DockContext);
  const reduce = useReducedMotion();
  const ref = useRef<HTMLDivElement | HTMLButtonElement>(null);

  const size = dock?.size ?? 44;
  const pillLayoutId = dock?.pillLayoutId ?? "dock-pill";
  const magnify = dock?.magnify ?? false;
  const maxScale = dock?.maxScale ?? 1.6;
  const reach = dock?.reach ?? 130;
  const pointerX = dock?.pointerX;

  // Distance from the pointer to this item's centre. Measured per frame rather
  // than cached: the dock is centred on the viewport, so every item moves when
  // its neighbours grow.
  const fallbackX = useMotionValue(Number.POSITIVE_INFINITY);
  const distance = useTransform(pointerX ?? fallbackX, (x) => {
    const bounds = ref.current?.getBoundingClientRect();
    if (!bounds) return Number.POSITIVE_INFINITY;
    return x - bounds.x - bounds.width / 2;
  });

  const target = useTransform(
    distance,
    [-reach, 0, reach],
    [size, size * maxScale, size],
  );
  const measure = useSpring(target, SPRING_MOUSE);
  const dimension = magnify ? measure : size;

  const pill = active ? (
    <motion.span
      layoutId={pillLayoutId}
      transition={reduce ? { duration: 0 } : SPRING_LAYOUT}
      className="absolute inset-0.5 -z-10 rounded-xl bg-primary/5"
    />
  ) : null;

  const sharedStyle = { width: dimension, height: dimension };
  const sharedClass = cn(
    "relative flex shrink-0 items-center justify-center rounded-full text-foreground",
    className,
  );

  if (onClick) {
    return (
      <motion.button
        ref={ref as React.Ref<HTMLButtonElement>}
        type="button"
        onClick={onClick}
        aria-label={rest["aria-label"]}
        aria-pressed={active}
        style={sharedStyle}
        className={cn(
          sharedClass,
          "cursor-pointer border-0 bg-transparent p-0 outline-none",
          "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
        )}
      >
        {pill}
        {children}
      </motion.button>
    );
  }

  return (
    <motion.div
      ref={ref as React.Ref<HTMLDivElement>}
      style={sharedStyle}
      className={sharedClass}
    >
      {pill}
      {children}
    </motion.div>
  );
}

export function DockSeparator({ className }: { className?: string }) {
  return (
    <span
      aria-hidden
      className={cn("mx-1 h-6 w-px self-center bg-border", className)}
    />
  );
}
