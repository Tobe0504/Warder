"use client";

import { useState } from "react";

import { Button } from "@/components/motion/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

/**
 * Confirmation before signing out.
 *
 * Signing out is cheap to undo, so this is not a guard against damage: it is a
 * guard against the miss. Sign out sits one item below Settings in the same
 * menu, and losing a half-filled dialog to a slipped click is the kind of
 * annoyance that has no upside.
 *
 * Controlled by the caller rather than owning a trigger, because every place
 * that offers sign-out reaches it through a menu that unmounts its own content
 * on close. The dialog has to be a sibling of that menu, not a child of it.
 */
export function SignOutDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const toast = useToast();
  const [pending, setPending] = useState(false);

  async function signOut() {
    setPending(true);
    try {
      await apiFetch("/api/auth/logout", { method: "POST" });
    } catch {
      // The redirect happens regardless. A revoke that failed surfaces as the
      // next request being refused, which is the same outcome from here.
    } finally {
      toast.success("Signed out.");
      // A full document load rather than a router push: the session is over,
      // and every cached server component payload the router is holding was
      // rendered for the person who just left.
      window.location.href = "/login";
    }
  }

  return (
    <Dialog open={open} onOpenChange={pending ? undefined : onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Sign out?</DialogTitle>
          <DialogDescription>
            This session ends now. Nothing is rotated and no runtime loses
            access; only this browser is signed out.
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline" disabled={pending}>
              Stay signed in
            </Button>
          </DialogClose>
          <Button onClick={signOut} loading={pending}>
            Sign out
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
