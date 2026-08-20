"use client";

import { Slot } from "@radix-ui/react-slot";
import {
  AnimatePresence,
  type HTMLMotionProps,
  motion,
  useReducedMotion,
} from "motion/react";

import { Loader } from "@/components/motion/loader";
import { SPRING_SWAP } from "@/lib/ease";
import {
  forwardRef,
  type PointerEvent,
  type ReactNode,
  type Ref,
  useCallback,
  useRef,
  useState,
} from "react";

import { EASE_OUT, SPRING_PRESS } from "@/lib/ease";
import { useHoverCapable } from "@/lib/hooks/use-hover-capable";
import { cn } from "@/lib/utils";

export type ButtonVariant =
  | "default"
  | "primary"
  | "secondary"
  | "ghost"
  | "outline"
  | "destructive"
  | "link";

export type ButtonSize = "default" | "sm" | "md" | "lg" | "icon";

const VARIANT_CLASS: Record<ButtonVariant, string> = {
  default: "bg-primary text-primary-foreground hover:bg-primary/90",
  primary: "bg-primary text-primary-foreground hover:bg-primary/90",
  secondary:
    "border border-border bg-secondary text-secondary-foreground hover:bg-muted",
  ghost: "text-muted-foreground hover:text-foreground hover:bg-muted",
  outline: "border border-input bg-transparent text-foreground hover:bg-muted",
  destructive:
    "bg-destructive text-destructive-foreground hover:bg-destructive/90",
  link: "text-foreground underline underline-offset-4 hover:opacity-70",
};

const SIZE_CLASS: Record<ButtonSize, string> = {
  default: "h-9 px-4 text-body gap-1.5 rounded-sm",
  sm: "h-8 px-3 text-meta gap-1.5 rounded-sm",
  md: "h-10 px-5 text-body gap-2 rounded-sm",
  lg: "h-11 px-6 text-body gap-2 rounded-sm",
  icon: "h-9 w-9 rounded-sm",
};

const BASE =
  "inline-flex items-center justify-center whitespace-nowrap font-medium select-none transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0";

export interface ButtonProps extends Omit<
  HTMLMotionProps<"button">,
  "children"
> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  pressScale?: number;
  ripple?: boolean;
  asChild?: boolean;
  /**
   * Show a spinner beside the label and refuse presses.
   *
   * The label does not change. A button that reads "Create project" before the
   * click and "Creating…" after it has swapped the name of the action for a
   * status, which loses the thing the reader was aiming at; the spinner is
   * already saying "in progress", and saying it twice costs the label.
   */
  loading?: boolean;
  children?: ReactNode;
}

type Ripple = { id: number; x: number; y: number; size: number };

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button(
    {
      variant = "default",
      size = "default",
      pressScale = 0.96,
      ripple = false,
      asChild = false,
      loading = false,
      className,
      children,
      onPointerDown,
      disabled,
      ...rest
    },
    ref,
  ) {
    const reduce = useReducedMotion();
    const canHover = useHoverCapable();
    const [ripples, setRipples] = useState<Ripple[]>([]);
    const nextId = useRef(0);

    const classes = cn(
      BASE,
      VARIANT_CLASS[variant],
      SIZE_CLASS[size],
      className,
    );

    const handlePointerDown = useCallback(
      (event: PointerEvent<HTMLButtonElement>) => {
        if (ripple && !reduce) {
          const rect = event.currentTarget.getBoundingClientRect();
          const spread = Math.max(rect.width, rect.height) * 2;
          const id = nextId.current++;
          setRipples((prev) => [
            ...prev,
            {
              id,
              x: event.clientX - rect.left,
              y: event.clientY - rect.top,
              size: spread,
            },
          ]);
        }
        onPointerDown?.(event);
      },
      [ripple, reduce, onPointerDown],
    );

    if (asChild) {
      return (
        <Slot
          ref={ref as Ref<HTMLElement>}
          className={cn(
            classes,
            "transition-[color,background-color,border-color,opacity,scale] active:scale-[0.96] motion-reduce:active:scale-100",
          )}
          {...(rest as Record<string, unknown>)}
        >
          {children}
        </Slot>
      );
    }

    return (
      <motion.button
        ref={ref}
        type="button"
        whileTap={reduce || loading ? undefined : { scale: pressScale }}
        whileHover={reduce || loading || !canHover ? undefined : { scale: 1.02 }}
        transition={SPRING_PRESS}
        onPointerDown={handlePointerDown}
        disabled={disabled || loading}
        aria-busy={loading || undefined}
        className={cn(classes, ripple && "relative overflow-hidden")}
        {...rest}
      >
        {ripple && !reduce ? (
          <span className="pointer-events-none absolute inset-0 overflow-hidden rounded-[inherit]">
            <AnimatePresence>
              {ripples.map((r) => (
                <motion.span
                  key={r.id}
                  className="absolute rounded-full bg-current"
                  style={{
                    left: r.x,
                    top: r.y,
                    width: r.size,
                    height: r.size,
                    x: "-50%",
                    y: "-50%",
                  }}
                  initial={{ scale: 0.05, opacity: 0.3 }}
                  animate={{ scale: 1, opacity: 0 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 1.6, ease: EASE_OUT }}
                  onAnimationComplete={() =>
                    setRipples((prev) => prev.filter((x) => x.id !== r.id))
                  }
                />
              ))}
            </AnimatePresence>
          </span>
        ) : null}

        {/*
          The spinner animates its own width from zero, so the label glides
          sideways to make room rather than being replaced or jumping. Overflow
          is clipped on the slot alone, so a ripple is unaffected.

          An icon-only button has no label to make room for, and a spinner
          beside a glyph in a 28px square would squash both, so there the
          spinner stands in for the icon instead.
        */}
        {size === "icon" ? (
          loading ? (
            <Loader variant="spinner" size={16} label="" className="text-current" />
          ) : (
            children
          )
        ) : (
          <>
            <AnimatePresence initial={false}>
              {loading ? (
                <motion.span
                  key="loader"
                  initial={{ width: 0, opacity: 0 }}
                  animate={{ width: "auto", opacity: 1 }}
                  exit={{ width: 0, opacity: 0 }}
                  transition={reduce ? { duration: 0 } : SPRING_SWAP}
                  className="inline-flex shrink-0 items-center overflow-hidden"
                >
                  <Loader
                    variant="spinner"
                    size={14}
                    label=""
                    className="me-1.5 text-current"
                  />
                </motion.span>
              ) : null}
            </AnimatePresence>
            {children}
          </>
        )}
      </motion.button>
    );
  },
);

export interface ButtonLinkProps extends Omit<
  HTMLMotionProps<"a">,
  "children"
> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  pressScale?: number;
  children?: ReactNode;
}

export const ButtonLink = forwardRef<HTMLAnchorElement, ButtonLinkProps>(
  function ButtonLink(
    {
      variant = "default",
      size = "default",
      pressScale = 0.96,
      className,
      children,
      ...rest
    },
    ref,
  ) {
    const reduce = useReducedMotion();
    const canHover = useHoverCapable();

    return (
      <motion.a
        ref={ref}
        whileTap={reduce ? undefined : { scale: pressScale }}
        whileHover={reduce || !canHover ? undefined : { scale: 1.02 }}
        transition={SPRING_PRESS}
        className={cn(
          BASE,
          VARIANT_CLASS[variant],
          SIZE_CLASS[size],
          className,
        )}
        {...rest}
      >
        {children}
      </motion.a>
    );
  },
);
