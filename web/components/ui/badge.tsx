import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-meta font-medium leading-none",
  {
    variants: {
      variant: {
        default: "border-transparent bg-secondary text-secondary-foreground",
        outline: "text-muted-foreground",
        destructive: "border-transparent bg-destructive/10 text-destructive",

        // The two variants that carry the product's meaning. They are visually
        // distinct from each other and from everything else, because "can use"
        // and "can see" are the distinction a person is scanning a page for.
        use: "border-transparent bg-can-use-surface text-can-use",
        reveal: "border-transparent bg-can-reveal-surface text-can-reveal",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export interface BadgeProps
  extends
    React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <span className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { Badge, badgeVariants };
