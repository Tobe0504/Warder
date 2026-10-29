"use client";

import { useState } from "react";
import { Check, Copy, LifeBuoy } from "lucide-react";

import { Button } from "@/components/motion/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { apiFetch } from "@/lib/client-api";

type Issued = { token: string; expiresAt: string };

/**
 * An owner issuing a one-time recovery link for a member.
 *
 * The last resort, for somebody who has lost their password, their passkeys
 * and their recovery codes. It is a real privilege: whoever opens the link sets
 * the password, so the dialog says that before anything is issued rather than
 * after.
 *
 * The token goes in the URL fragment, which browsers never send to a server,
 * so the link stays out of access logs and Referer headers.
 */
export function IssueRecoveryDialog({ name, membershipId }: { name: string; membershipId: string }) {
  const [open, setOpen] = useState(false);
  const [issued, setIssued] = useState<Issued | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [copied, setCopied] = useState(false);

  const link = issued ? `${window.location.origin}/recover#${issued.token}` : "";

  function close() {
    setOpen(false);
    setIssued(null);
    setError(null);
    setCopied(false);
  }

  async function issue() {
    setPending(true);
    setError(null);
    try {
      setIssued(
        await apiFetch<Issued>(`/api/members/${membershipId}/recovery`, { method: "POST" }),
      );
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not issue a recovery link.");
    } finally {
      setPending(false);
    }
  }

  async function copy() {
    await navigator.clipboard.writeText(link);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? setOpen(true) : close())}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-7"
          aria-label={`Issue a recovery link for ${name}`}
          title="Issue a recovery link"
        >
          <LifeBuoy />
        </Button>
      </DialogTrigger>

      <DialogContent>
        {issued ? (
          <>
            <DialogHeader>
              <DialogTitle>Recovery link ready</DialogTitle>
              <DialogDescription>
                Send this to {name} over a channel you already trust. It works once and expires{" "}
                {new Date(issued.expiresAt).toLocaleTimeString([], {
                  hour: "numeric",
                  minute: "2-digit",
                })}
                .
              </DialogDescription>
            </DialogHeader>

            <div className="flex items-center gap-2 rounded-md border bg-muted px-2.5 py-2">
              <code className="min-w-0 flex-1 break-all font-sans text-meta">{link}</code>
              <Button variant="ghost" size="icon" onClick={copy} aria-label="Copy recovery link">
                {copied ? <Check /> : <Copy />}
              </Button>
            </div>

            <p className="mt-2 prose-note text-muted-foreground">
              Only a verifier is stored, so this can&apos;t be shown again. Issuing another replaces
              this one.
            </p>

            <DialogFooter>
              <Button onClick={close}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Recover {name}&apos;s account?</DialogTitle>
              <DialogDescription>
                For someone who has lost their password, passkeys and recovery codes. Whoever opens
                the link sets a new password, and all of {name}&apos;s passkeys and sessions are
                removed.
              </DialogDescription>
            </DialogHeader>

            <p className="prose-note text-muted-foreground">
              Only send it once you&apos;ve confirmed who you&apos;re talking to. Anyone holding the
              link can take over the account, and this is recorded in the audit log.
            </p>

            {error && (
              <p role="alert" className="text-meta text-destructive">
                {error}
              </p>
            )}

            <DialogFooter>
              <Button variant="ghost" onClick={close} disabled={pending}>
                Cancel
              </Button>
              <Button onClick={issue} loading={pending}>
                Issue link
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
