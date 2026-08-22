"use client";

import { useState } from "react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

/**
 * Changes when a secret expires, without minting a new version.
 *
 * The alternative was rotating, which is what the batch importer tells people
 * to do when a key already exists. That is right when the value is compromised
 * and wrong when only the date lapsed: it forces someone to invent a new
 * credential to correct a calendar entry, and every runtime holding the old one
 * has to be restarted for no reason.
 *
 * Gated on the same capability as rotation, ROTATE_SECRET, because extending an
 * expiry keeps a live credential alive. The server checks again regardless.
 */
export function ChangeExpiryDialog({
  secretId,
  secretKey,
  currentExpiry,
  onChanged,
  children,
}: {
  secretId: string;
  secretKey: string;
  currentExpiry: string | null;
  onChanged?: () => void;
  children: React.ReactNode;
}) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState(() => toLocalInput(currentExpiry));

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setPending(true);
    setError(null);

    try {
      await apiFetch(`/api/secrets/${secretId}/expiry`, {
        method: "POST",
        body: { expiresAt: expiresAt ? new Date(expiresAt).toISOString() : "" },
      });
      toast.success(
        expiresAt
          ? `${secretKey} now expires ${new Date(expiresAt).toLocaleString()}.`
          : `${secretKey} no longer expires.`,
      );
      setOpen(false);
      onChanged?.();
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "Could not change the expiry.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={pending ? undefined : setOpen}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>Change expiry</DialogTitle>
            <DialogDescription>
              The value does not change and nothing restarts. Running
              applications keep the credential they already hold.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3 py-4">
            <Input
              label={`New expiry for ${secretKey}`}
              type="datetime-local"
              value={expiresAt}
              onChange={setExpiresAt}
            />
            <p className="text-meta text-muted-foreground">
              Leave empty for no expiry. A credential that never expires is one
              nobody is ever reminded to review.
            </p>

            {error && (
              <p role="alert" className="text-meta text-destructive">
                {error}
              </p>
            )}
          </div>

          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={pending}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" loading={pending}>
              Save expiry
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Formats an ISO timestamp for a datetime-local input, which wants local time
 * with no zone and no seconds.
 */
function toLocalInput(iso: string | null): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}
