"use client";

import * as React from "react";
import { Check, ChevronDown, Search, X } from "lucide-react";

import { Button } from "@/components/motion/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

/**
 * The filter row above a list, in the shape Vercel uses on its deployments
 * table: a search field, then a row of dropdowns that each read "All <thing>"
 * until something is chosen.
 *
 * "All X" as the resting label matters. A dropdown showing a blank or a
 * placeholder leaves you unsure whether a filter is applied; "All environments"
 * states plainly that nothing is being hidden from you, which on a screen
 * about access is worth being unambiguous about.
 */

export function FilterBar({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("mb-3 flex flex-wrap items-center gap-2", className)} {...props} />
  );
}

export function FilterSearch({
  value,
  onChange,
  placeholder = "Search…",
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}) {
  const inputRef = React.useRef<HTMLInputElement>(null);

  // "/" focuses search, the convention this kind of dense list has settled on.
  React.useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typing =
        target instanceof HTMLInputElement ||
        target instanceof HTMLTextAreaElement ||
        target?.isContentEditable;

      if (event.key === "/" && !typing) {
        event.preventDefault();
        inputRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  return (
    <div className={cn("relative min-w-[12rem] flex-1 sm:max-w-xs", className)}>
      <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <input
        ref={inputRef}
        type="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
        className={cn(
          "h-9 w-full rounded-md border border-input bg-transparent pl-9 pr-9 text-meta",
          "placeholder:text-muted-foreground",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
          "[&::-webkit-search-cancel-button]:appearance-none",
        )}
      />
      {value ? (
        <button
          onClick={() => onChange("")}
          aria-label="Clear search"
          className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground transition-opacity hover:opacity-70"
        >
          <X className="size-3.5" />
        </button>
      ) : (
        <kbd className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rounded border px-1 font-mono text-meta text-muted-foreground">
          /
        </kbd>
      )}
    </div>
  );
}

export type FilterOption = { value: string; label: string; hint?: string };

export function FilterSelect({
  label,
  value,
  options,
  onChange,
  className,
}: {
  /** Shown when nothing is selected, as "All <label>". */
  label: string;
  value: string;
  options: FilterOption[];
  onChange: (value: string) => void;
  className?: string;
}) {
  const selected = options.find((option) => option.value === value);
  const active = value !== "";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn(
            "h-9 gap-1.5 px-3 font-normal",
            active && "border-foreground/25 bg-muted",
            className,
          )}
        >
          <span className={cn("text-meta", !active && "text-muted-foreground")}>
            {selected ? selected.label : `All ${label}`}
          </span>
          <ChevronDown className="text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="start" className="w-52">
        <DropdownMenuLabel>{label}</DropdownMenuLabel>
        <DropdownMenuSeparator />

        <DropdownMenuItem onSelect={() => onChange("")}>
          <span className="flex size-3.5 items-center justify-center">
            {!active && <Check className="size-3.5" />}
          </span>
          All {label}
        </DropdownMenuItem>

        {options.map((option) => (
          <DropdownMenuItem key={option.value} onSelect={() => onChange(option.value)}>
            <span className="flex size-3.5 items-center justify-center">
              {value === option.value && <Check className="size-3.5" />}
            </span>
            <span className="flex-1 truncate">{option.label}</span>
            {option.hint && (
              <span className="font-mono text-meta text-muted-foreground">{option.hint}</span>
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Clears every active filter at once. */
export function FilterReset({ show, onReset }: { show: boolean; onReset: () => void }) {
  if (!show) return null;
  return (
    <Button variant="ghost" size="sm" onClick={onReset} className="h-9 gap-1.5 px-2.5 font-normal">
      <X />
      <span className="text-meta">Reset</span>
    </Button>
  );
}
