"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Bot, GitBranch, Plus, Server, Workflow } from "lucide-react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";
import { cn } from "@/lib/utils";

/**
 * The kinds of machine identity, chosen as cards rather than a dropdown.
 *
 * The type is descriptive — it changes nothing about how policy is evaluated —
 * but it changes how a person reads the access screen later. Making the choice
 * visual, with a sentence about what each is for, means the audit trail ends up
 * with accurate labels instead of everything being "service".
 */
const KINDS = [
  {
    value: "WORKLOAD",
    label: "Workload",
    icon: Server,
    hint: "A running application: an API, a worker, a container.",
  },
  {
    value: "CI",
    label: "CI pipeline",
    icon: GitBranch,
    hint: "A build or deploy job.",
  },
  {
    value: "AI_AGENT",
    label: "AI agent",
    icon: Bot,
    hint: "A coding agent session. Give it development scope and an expiry.",
  },
  {
    value: "SERVICE",
    label: "Service",
    icon: Workflow,
    hint: "Anything else that runs unattended.",
  },
] as const;

export function CreateIdentityDialog() {
  const router = useRouter();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [actorType, setActorType] = useState<string>("WORKLOAD");
  const [expiresAt, setExpiresAt] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const isAgent = actorType === "AI_AGENT";

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError(null);

    try {
      await apiFetch("/api/identities", {
        method: "POST",
        body: {
          name,
          actorType,
          expiresAt: expiresAt ? new Date(expiresAt).toISOString() : "",
        },
      });
      setOpen(false);
      setName("");
      setExpiresAt("");
      toast.success(`${name} created. It holds no access until you grant it some.`);
      router.refresh();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not create this identity.");
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus />
          New identity
        </Button>
      </DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>New identity</DialogTitle>
          <DialogDescription>
            An identity is a thing that runs your code. It starts with no access at all.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="identity-name">Name</Label>
            <Input
              id="identity-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="payments-api"
              required
            />
          </div>

          <fieldset className="space-y-1.5">
            <legend className="mb-1.5 text-meta font-medium text-muted-foreground">Kind</legend>
            <div className="grid grid-cols-2 gap-1.5">
              {KINDS.map((kind) => {
                const Icon = kind.icon;
                const selected = actorType === kind.value;
                return (
                  <label
                    key={kind.value}
                    className={cn(
                      "cursor-pointer rounded-md border p-2 transition-colors",
                      selected
                        ? "border-foreground/25 bg-muted"
                        : "border-border hover:border-foreground/15",
                    )}
                  >
                    <input
                      type="radio"
                      name="actorType"
                      value={kind.value}
                      checked={selected}
                      onChange={(e) => setActorType(e.target.value)}
                      className="sr-only"
                    />
                    <span className="flex items-center gap-1.5 text-meta font-medium">
                      <Icon className="size-3.5" />
                      {kind.label}
                    </span>
                    <span className="mt-0.5 block text-meta leading-snug text-muted-foreground">
                      {kind.hint}
                    </span>
                  </label>
                );
              })}
            </div>
          </fieldset>

          <div className="space-y-1.5">
            <Label htmlFor="identity-expiry">
              Expires {isAgent ? "(recommended)" : "(optional)"}
            </Label>
            <Input
              id="identity-expiry"
              type="datetime-local"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
            />
            {isAgent && (
              // Surfaced only for agents, where it is the difference between a
              // session that ends on its own and one that lingers until someone
              // remembers it exists.
              <p className="prose-note text-muted-foreground">
                An agent session should end by itself. When this passes, every token issued to this
                identity stops working — no cleanup needed.
              </p>
            )}
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
              {pending ? "Creating…" : "Create identity"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
