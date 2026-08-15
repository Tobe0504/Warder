"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

export function RotateSecretDialog({
  secretId,
  secretKey,
  onRotated,
  children,
}: {
  secretId: string;
  secretKey: string;
  onRotated: () => void;
  children: React.ReactNode;
}) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError(null);

    try {
      await apiFetch(`/api/secrets/${secretId}/rotate`, { method: "POST", body: { value } });
      setOpen(false);
      toast.success(`${secretKey} rotated. Running applications pick it up on their next start.`);
      onRotated();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not rotate this secret.");
    } finally {
      setValue("");
      setPending(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setValue("");
          setError(null);
        }
      }}
    >
      <DialogTrigger asChild>{children}</DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            Rotate <span className="font-mono">{secretKey}</span>
          </DialogTitle>
          <DialogDescription>
            A new version becomes active immediately. Applications keep referring to the same key
            and receive the new value on their next start — nothing needs reconfiguring.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="rotate-value">New value</Label>
            <Textarea
              id="rotate-value"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              className="font-mono text-meta"
              autoComplete="off"
              spellCheck={false}
              required
            />
          </div>

          {/*
            Stated plainly, because the opposite assumption is dangerous: an
            operator who believes rotating here also rotated the credential at
            the provider will leave a live credential in place.
          */}
          <p className="rounded-md bg-muted px-2.5 py-2 prose-note text-muted-foreground">
            This changes the value Warder stores. It does not change the credential at the
            provider — rotate it there first, then paste the new one here.
          </p>

          {error && (
            <p role="alert" className="text-meta text-destructive">
              {error}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={pending}>
              {pending ? "Rotating…" : "Rotate"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
