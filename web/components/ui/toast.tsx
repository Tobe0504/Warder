"use client";

import * as React from "react";

import {
  AnimatedToastStack,
  useAnimatedToastStack,
} from "@/components/motion/animated-toast-stack";

/**
 * Toasts.
 *
 * The queue and the presentation are beUI's; this file is the seam that keeps
 * the rest of the application on a two-method API. `toast.success(string)` and
 * `toast.error(string)` accept a string and nothing else, so no call site can
 * hand the renderer an object that stringifies into something it should not,
 * and React escapes what does get through. Thirteen components call this, and
 * none of them should have to know how a toast is drawn.
 */

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
  const { toasts, showToast, dismissToast } = useAnimatedToastStack({
    defaultDuration: SUCCESS_MS,
  });

  const value = React.useMemo<ToastContextValue>(
    () => ({
      success: (message: string) =>
        showToast({ title: message, status: "success", duration: SUCCESS_MS }),
      error: (message: string) =>
        showToast({ title: message, status: "error", duration: ERROR_MS }),
    }),
    [showToast],
  );

  return (
    <ToastContext.Provider value={value}>
      {children}

      {/*
        'polite' rather than 'assertive': a confirmation should not interrupt
        whatever a screen reader is in the middle of saying. The stack renders
        into a portal, so the live region has to wrap it here.
      */}
      <div aria-live="polite" aria-atomic="false">
        <AnimatedToastStack
          toasts={toasts}
          onDismiss={dismissToast}
          position="bottom-right"
          maxVisible={3}
        />
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
