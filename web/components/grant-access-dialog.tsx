"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { AlertTriangle, ShieldPlus } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Checkbox } from "@/components/motion/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/motion/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/motion/select";
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
            <Label>For</Label>
            <Select value={subject} onValueChange={setSubject}>
              <SelectTrigger>
                <SelectValue placeholder="Choose an identity…" />
              </SelectTrigger>
              <SelectContent>
                {members.map((member) => (
                  <SelectItem key={member.userId} value={`USER:${member.userId}`}>
                    {`${member.name} · ${member.email}`}
                  </SelectItem>
                ))}
                {identities.map((identity) => (
                  <SelectItem key={identity.id} value={`MACHINE:${identity.id}`}>
                    {`${identity.name} · ${identity.actorType.toLowerCase().replace("_", " ")}`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label>Environment</Label>
            <Select value={environmentId} onValueChange={setEnvironmentId}>
              <SelectTrigger>
                <SelectValue placeholder="Choose an environment…" />
              </SelectTrigger>
              <SelectContent>
                {environments.map((environment) => (
                  <SelectItem key={environment.id} value={environment.id}>
                    {environment.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <fieldset className="space-y-2 rounded-md border p-2.5">
            <legend className="px-1 text-meta font-medium text-muted-foreground">Capabilities</legend>

            <div className="flex items-start gap-2">
              <Checkbox
                checked={canUse}
                onCheckedChange={setCanUse}
                aria-label="Can use"
                className="mt-0.5"
              />
              <span className="text-meta">
                <span className="font-medium">Can use</span>
                <span className="block text-meta text-muted-foreground">
                  A runtime may receive the value and inject it into a process. The person never
                  sees it.
                </span>
              </span>
            </div>

            <div className="flex items-start gap-2">
              <Checkbox
                checked={canReveal}
                onCheckedChange={setCanReveal}
                aria-label="Can see"
                className="mt-0.5"
              />
              <span className="text-meta">
                <span className="font-medium">Can see</span>
                <span className="block text-meta text-muted-foreground">
                  The plaintext can be displayed in this dashboard. Every disclosure is recorded.
                </span>
              </span>
            </div>
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
            <Input
              id="grant-expiry"
              label={`Expires ${canReveal ? "(recommended)" : "(optional)"}`}
              type="datetime-local"
              value={expiresAt}
              onChange={setExpiresAt}
            />
          </div>

          <div className="space-y-1.5">
            <Input
              id="grant-reason"
              label={`Reason ${canReveal ? "(required)" : "(optional)"}`}
              value={reason}
              onChange={setReason}
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
            <Button type="submit" loading={pending}>
              Grant access
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
