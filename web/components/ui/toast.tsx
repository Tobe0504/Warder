"use client";

import * as React from "react";
import { AlertTriangle, Check, X } from "lucide-react";

import { cn } from "@/lib/utils";

/**
 * Toasts.
 *
 * Written rather than pulled from a library, for two reasons. The dependency
 * budget on a security tool is worth spending carefully, and, more to the
 * point: a general toast library will happily render whatever it is handed,
 * including an object that stringifies to something it should not. This one
 * accepts a string and nothing else, and React escapes it.
 *
 * Announced through an aria-live region so that a screen reader hears the
 * result of an action rather than nothing happening.
 */

type ToastTone = "success" | "error";

type Toast = {
  id: number;
  tone: ToastTone;
  message: string;
};

type ToastContextValue = {
  success: (message: string) => void;
  error: (message: string) => void;
};

const ToastContext = React.createContext<ToastContextValue | null>(null);

/** How long a toast stays before dismissing itself. */
const SUCCESS_MS = 4000;
// Errors linger: they carry something the reader may need to act on, and a
// message that vanishes before it is read is worse than none.
const ERROR_MS = 8000;

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = React.useState<Toast[]>([]);
  const nextId = React.useRef(0);

  const dismiss = React.useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const push = React.useCallback(
    (tone: ToastTone, message: string) => {
      const id = nextId.current++;
      setToasts((current) => [...current, { id, tone, message }]);
      setTimeout(() => dismiss(id), tone === "error" ? ERROR_MS : SUCCESS_MS);
    },
    [dismiss],
  );

  const value = React.useMemo<ToastContextValue>(
    () => ({
      success: (message: string) => push("success", message),
      error: (message: string) => push("error", message),
    }),
    [push],
  );

  return (
    <ToastContext.Provider value={value}>
      {children}

      <div
        // 'polite' rather than 'assertive': a confirmation should not interrupt
        // whatever a screen reader is in the middle of saying.
        aria-live="polite"
        aria-atomic="false"
        className="pointer-events-none fixed bottom-3 right-3 z-50 flex w-full max-w-xs flex-col gap-2"
      >
        {toasts.map((toast) => (
          <div
            key={toast.id}
            role={toast.tone === "error" ? "alert" : "status"}
            className={cn(
              "pointer-events-auto flex items-start gap-2 rounded-lg border bg-card px-3 py-2.5 shadow-lg",
              "duration-150 animate-in fade-in-0 slide-in-from-bottom-2",
              toast.tone === "error" && "border-destructive/30",
            )}
          >
            {toast.tone === "success" ? (
              <Check className="mt-0.5 size-3.5 shrink-0 text-can-use" />
            ) : (
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-destructive" />
            )}

            <p className="flex-1 text-meta leading-relaxed">{toast.message}</p>

            <button
              onClick={() => dismiss(toast.id)}
              aria-label="Dismiss"
              className="rounded-sm text-muted-foreground transition-opacity hover:opacity-70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <X className="size-3.5" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

/**
 * Access to the toast queue.
 *
 * Falls back to a no-op outside a provider rather than throwing, so that a
 * component rendered in isolation, a test, a future storybook, does not crash
 * over a notification.
 */
export function useToast(): ToastContextValue {
  const context = React.useContext(ToastContext);
  return context ?? { success: () => {}, error: () => {} };
}
