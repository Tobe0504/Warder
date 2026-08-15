"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, KeyRound, Loader2, X } from "lucide-react";

import { apiFetch } from "@/lib/client-api";
import { cn } from "@/lib/utils";

type SecretSummary = { id: string; key: string };

/**
 * Chooses which secrets a token may reach.
 *
 * A multi-select combobox rather than a free-text field. Narrowing a token to
 * specific keys is a least-privilege decision, and it only works if the names
 * are exactly right, a typo does not fail loudly, it scopes the token to a
 * secret that does not exist and the workload gets nothing at runtime for a
 * reason invisible from this dialog. So the options come from the environment
 * that was actually selected, and are picked rather than typed.
 *
 * Choosing none remains the default and still means "everything this identity
 * is granted here": the narrowing is opt-in.
 */
export function SecretKeyPicker({
  environmentId,
  selected,
  onChange,
}: {
  environmentId: string;
  selected: string[];
  onChange: (keys: string[]) => void;
}) {
  const [keys, setKeys] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!environmentId) {
      setKeys([]);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    apiFetch<{ secrets: SecretSummary[] | null }>(
      `/api/environments/${environmentId}/secrets`,
    )
      .then((result) => {
        if (cancelled) return;
        const available = (result.secrets ?? []).map((secret) => secret.key);
        setKeys(available);
        // Drop anything selected that this environment does not have; it would
        // scope the token to a key that cannot resolve.
        onChange(selected.filter((key) => available.includes(key)));
      })
      .catch((caught) => {
        if (!cancelled)
          setError(
            caught instanceof Error
              ? caught.message
              : "Could not load secrets.",
          );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
    // `selected` is written by this effect, so including it would loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [environmentId]);

  // Close on an outside click, the behaviour a combobox is expected to have.
  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  const matches = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return keys.filter((key) => !needle || key.toLowerCase().includes(needle));
  }, [keys, query]);

  function toggle(key: string) {
    onChange(
      selected.includes(key)
        ? selected.filter((k) => k !== key)
        : [...selected, key],
    );
    setQuery("");
    inputRef.current?.focus();
  }

  function onKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Backspace" && query === "" && selected.length > 0) {
      onChange(selected.slice(0, -1));
      return;
    }
    if (event.key === "Enter" && matches.length > 0) {
      // Not a submit: the first match is chosen instead, so Enter never
      // submits the dialog while someone is still filtering.
      event.preventDefault();
      const first = matches.find((key) => !selected.includes(key));
      if (first) toggle(first);
      return;
    }
    if (event.key === "Escape") setOpen(false);
    if (event.key === "ArrowDown") setOpen(true);
  }

  if (loading) {
    return (
      <div className="flex h-9 items-center gap-2 rounded-md border px-3 text-meta text-muted-foreground">
        <Loader2 className="size-3.5 animate-spin" />
        Loading secrets…
      </div>
    );
  }

  if (error) {
    return (
      <p role="alert" className="text-meta text-destructive">
        {error}
      </p>
    );
  }

  if (keys.length === 0) {
    return (
      <div className="flex h-9 items-center gap-2 rounded-md border px-3 text-meta text-muted-foreground">
        <KeyRound className="size-3.5" />
        This environment has no secrets yet.
      </div>
    );
  }

  return (
    <div ref={rootRef} className="relative">
      <div
        onClick={() => {
          setOpen(true);
          inputRef.current?.focus();
        }}
        className={cn(
          "flex min-h-9 w-full flex-wrap items-center gap-1.5 rounded-md border border-input px-2 py-1.5",
          "cursor-text transition-colors",
          open && "ring-2 ring-ring ring-offset-1 ring-offset-background",
        )}
      >
        {selected.map((key) => (
          <span
            key={key}
            className="inline-flex items-center gap-1 rounded bg-muted py-0.5 pl-2 pr-1 font-mono text-meta"
          >
            {key}
            <button
              type="button"
              aria-label={`Remove ${key}`}
              onClick={(event) => {
                event.stopPropagation();
                onChange(selected.filter((k) => k !== key));
              }}
              className="rounded text-muted-foreground transition-opacity hover:opacity-60"
            >
              <X className="size-3" />
            </button>
          </span>
        ))}

        <input
          ref={inputRef}
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setOpen(true);
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={onKeyDown}
          placeholder={selected.length === 0 ? "All granted secrets" : ""}
          aria-label="Filter secrets"
          role="combobox"
          aria-expanded={open}
          className="h-6 min-w-24 flex-1 bg-transparent font-mono text-meta outline-none placeholder:font-sans placeholder:text-muted-foreground rounded-sm "
        />

        <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
      </div>

      {open && (
        <div
          role="listbox"
          className="absolute z-50 mt-1 max-h-48 w-full overflow-y-auto rounded-lg border bg-card p-1 shadow-lg"
        >
          {matches.length === 0 ? (
            <p className="px-2 py-1.5 text-meta text-muted-foreground">
              No secret matches that.
            </p>
          ) : (
            matches.map((key) => {
              const checked = selected.includes(key);
              return (
                <button
                  key={key}
                  type="button"
                  role="option"
                  aria-selected={checked}
                  onClick={() => toggle(key)}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left transition-colors",
                    checked ? "bg-muted" : "hover:bg-muted/60",
                  )}
                >
                  <span
                    aria-hidden="true"
                    className={cn(
                      "flex size-4 shrink-0 items-center justify-center rounded border",
                      checked &&
                        "border-foreground bg-foreground text-background",
                    )}
                  >
                    {checked && <Check className="size-3" />}
                  </span>
                  <span className="truncate font-mono text-meta">{key}</span>
                </button>
              );
            })
          )}
        </div>
      )}

      {/* Only worth saying once a choice has been made: the placeholder
          already covers the empty case, and the dialog's own note covers what
          empty means. */}
      {selected.length > 0 && (
        <p className="mt-1.5 text-meta text-muted-foreground tabular">
          {selected.length} of {keys.length} selected
        </p>
      )}
    </div>
  );
}
