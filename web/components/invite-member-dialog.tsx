"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Check, Copy, Link2, UserPlus } from "lucide-react";

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
import { Input, Select } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiFetch } from "@/lib/client-api";

type Created = { inviteUrl: string; email: string; role: string; expiresAt: string };

const ROLES = [
  ["DEVELOPER", "Developer", "Can see what exists. Cannot administer anything."],
  ["VIEWER", "Viewer", "Read-only on metadata."],
  ["ADMIN", "Admin", "Manages projects, access, and identities."],
  ["OWNER", "Owner", "Everything an admin can do, plus organization settings."],
] as const;

/**
 * Invites somebody to the organization.
 *
 * Deliberately not a form that sets their password. An owner who chooses
 * somebody else's credential then has to transmit it, knows it afterwards, and
 * can sign in as them, which is a strange thing for a product built on the
 * idea that people should not hold credentials they do not need.
 *
 * So this produces a single-use link instead. The invitee sets their own
 * password, and the owner never learns it.
 */
export function InviteMemberDialog() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<string>("DEVELOPER");
  const [expiresAt, setExpiresAt] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  /** The link, held only until this dialog closes. */
  const [created, setCreated] = useState<Created | null>(null);
  const [copied, setCopied] = useState(false);

  function close() {
    setOpen(false);
    setCreated(null);
    setCopied(false);
    setName("");
    setEmail("");
    setRole("DEVELOPER");
    setExpiresAt("");
    setError(null);
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError(null);

    try {
      const result = await apiFetch<Created>("/api/members/invitations", {
        method: "POST",
        body: {
          name,
          email,
          role,
          expiresAt: expiresAt ? new Date(expiresAt).toISOString() : "",
        },
      });
      setCreated(result);
      router.refresh();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not create this invitation.");
    } finally {
      setPending(false);
    }
  }

  async function copy() {
    if (!created) return;
    await navigator.clipboard.writeText(created.inviteUrl);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? setOpen(true) : close())}>
      <DialogTrigger asChild>
        <Button size="sm">
          <UserPlus />
          Invite member
        </Button>
      </DialogTrigger>

      <DialogContent>
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>Invitation ready</DialogTitle>
              <DialogDescription>
                Send this link to {created.email}. It works once, expires in seven days, and can be
                withdrawn from this page until it is used.
              </DialogDescription>
            </DialogHeader>

            <div className="flex items-center gap-2 rounded-md border bg-muted px-2.5 py-2">
              <Link2 className="size-3.5 shrink-0 text-muted-foreground" />
              <code className="min-w-0 flex-1 break-all font-mono text-meta">
                {created.inviteUrl}
              </code>
              <Button variant="ghost" size="icon" onClick={copy} aria-label="Copy invitation link">
                {copied ? <Check /> : <Copy />}
              </Button>
            </div>

            <p className="mt-2 prose-note text-muted-foreground">
              Only a verifier is stored, so this cannot be shown again. They will set their own
              password: you will not know it, and you will not be able to sign in as them.
            </p>

            <DialogFooter>
              <Button onClick={close}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Invite a member</DialogTitle>
              <DialogDescription>
                You will get a link to send. Nothing is emailed from here.
              </DialogDescription>
            </DialogHeader>

            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="invite-name">Name</Label>
                <Input
                  id="invite-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Ada Lovelace"
                  required
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="invite-email">Email</Label>
                <Input
                  id="invite-email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="ada@example.com"
                  required
                />
                <p className="prose-note text-muted-foreground">
                  Fixed at this point. Whoever opens the link joins as this address and cannot
                  change it.
                </p>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="invite-role">Role</Label>
                <Select
                  id="invite-role"
                  value={role}
                  onChange={(e) => setRole(e.target.value)}
                  required
                >
                  {ROLES.map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </Select>
                <p className="prose-note text-muted-foreground">
                  {ROLES.find(([value]) => value === role)?.[2]} No role grants{" "}
                  <span className="font-mono">USE_SECRET</span> or{" "}
                  <span className="font-mono">READ_SECRET</span>: those are separate grants on a
                  project.
                </p>
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="invite-expiry">Membership ends (optional)</Label>
                <Input
                  id="invite-expiry"
                  type="datetime-local"
                  value={expiresAt}
                  onChange={(e) => setExpiresAt(e.target.value)}
                />
                <p className="prose-note text-muted-foreground">
                  The contractor case: set a date now and their access ends on its own, with
                  nothing to rotate and nobody to remind.
                </p>
              </div>

              {error && (
                <p role="alert" className="text-meta text-destructive">
                  {error}
                </p>
              )}

              <DialogFooter>
                <Button type="button" variant="ghost" onClick={close}>
                  Cancel
                </Button>
                <Button type="submit" disabled={pending}>
                  {pending ? "Creating…" : "Create invitation"}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
