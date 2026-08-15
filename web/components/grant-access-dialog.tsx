"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { AlertTriangle, ShieldPlus } from "lucide-react";

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
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

type Environment = { id: string; name: string; slug: string };
type Identity = { id: string; name: string; actorType: string };
type Member = { userId: string; name: string; email: string; role: string };

/**
 * Grants access.
 *
 * The two capabilities are presented as separate checkboxes with plain-language
 * descriptions rather than as a single "access level" dropdown. Collapsing them
 * into one control would be the moment the product's distinction quietly
 * disappeared from the interface.
 */
export function GrantAccessDialog({
  projectId,
  environments,
  identities,
  members,
}: {
  projectId: string;
  environments: Environment[];
  identities: Identity[];
  members: Member[];
}) {
  const router = useRouter();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [subject, setSubject] = useState("");
  const [environmentId, setEnvironmentId] = useState(environments[0]?.id ?? "");
  const [canUse, setCanUse] = useState(true);
  const [canReveal, setCanReveal] = useState(false);
  const [expiresAt, setExpiresAt] = useState("");
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    const [subjectType, subjectId] = subject.split(":");
    if (!subjectType || !subjectId) {
      setError("Choose who this grant is for.");
      return;
    }

    const capabilities: string[] = [];
    if (canUse) capabilities.push("USE_SECRET");
    if (canReveal) capabilities.push("READ_SECRET");
    if (capabilities.length === 0) {
      setError("Choose at least one capability.");
      return;
    }
    if (canReveal && reason.trim() === "") {
      setError("Give a reason when granting permission to see values.");
      return;
    }

    setPending(true);
    try {
      await apiFetch(`/api/projects/${projectId}/access`, {
        method: "POST",
        body: {
          subjectType,
          subjectId,
          environmentId,
          capabilities,
          expiresAt: expiresAt ? new Date(expiresAt).toISOString() : "",
          reason: reason.trim(),
        },
      });
      setOpen(false);
      setReason("");
      setCanReveal(false);
      toast.success(
        canReveal
          ? "Access granted, including permission to see values. This is recorded in the audit trail."
          : "Access granted. The value stays hidden.",
      );
      router.refresh();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not grant access.");
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm">
          <ShieldPlus />
          Grant access
        </Button>
      </DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>Grant access</DialogTitle>
          <DialogDescription>
            Using a credential and seeing it are separate permissions. Grant only what is needed.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="grant-subject">For</Label>
            <Select
              id="grant-subject"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              required
            >
              <option value="">Choose an identity…</option>
              {members.length > 0 && (
                <optgroup label="People">
                  {members.map((member) => (
                    <option key={member.userId} value={`USER:${member.userId}`}>
                      {member.name} · {member.email}
                    </option>
                  ))}
                </optgroup>
              )}
              {identities.length > 0 && (
                <optgroup label="Machines and agents">
                  {identities.map((identity) => (
                    <option key={identity.id} value={`MACHINE:${identity.id}`}>
                      {identity.name} · {identity.actorType.toLowerCase().replace("_", " ")}
                    </option>
                  ))}
                </optgroup>
              )}
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="grant-environment">Environment</Label>
            <Select
              id="grant-environment"
              value={environmentId}
              onChange={(e) => setEnvironmentId(e.target.value)}
              required
            >
              {environments.map((environment) => (
                <option key={environment.id} value={environment.id}>
                  {environment.name}
                </option>
              ))}
            </Select>
          </div>

          <fieldset className="space-y-2 rounded-md border p-2.5">
            <legend className="px-1 text-meta font-medium text-muted-foreground">Capabilities</legend>

            <label className="flex cursor-pointer items-start gap-2">
              <input
                type="checkbox"
                checked={canUse}
                onChange={(e) => setCanUse(e.target.checked)}
                className="mt-0.5 accent-[var(--can-use)]"
              />
              <span className="text-meta">
                <span className="font-medium">Can use</span>
                <span className="block text-meta text-muted-foreground">
                  A runtime may receive the value and inject it into a process. The person never
                  sees it.
                </span>
              </span>
            </label>

            <label className="flex cursor-pointer items-start gap-2">
              <input
                type="checkbox"
                checked={canReveal}
                onChange={(e) => setCanReveal(e.target.checked)}
                className="mt-0.5 accent-[var(--can-reveal)]"
              />
              <span className="text-meta">
                <span className="font-medium">Can see</span>
                <span className="block text-meta text-muted-foreground">
                  The plaintext can be displayed in this dashboard. Every disclosure is recorded.
                </span>
              </span>
            </label>
          </fieldset>

          {canReveal && (
            <div className="flex gap-2 rounded-md bg-can-reveal-surface px-2.5 py-2">
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-can-reveal" />
              <p className="prose-note text-can-reveal">
                This lets a person read the credential itself. Prefer a time limit, and say why:
                both appear in the audit trail.
              </p>
            </div>
          )}

          <div className="space-y-1.5">
            <Label htmlFor="grant-expiry">
              Expires {canReveal ? "(recommended)" : "(optional)"}
            </Label>
            <Input
              id="grant-expiry"
              type="datetime-local"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="grant-reason">Reason {canReveal ? "(required)" : "(optional)"}</Label>
            <Input
              id="grant-reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Debugging a failed migration"
            />
          </div>

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
              {pending ? "Granting…" : "Grant access"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
