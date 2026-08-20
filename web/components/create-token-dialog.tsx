"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Check, Copy, Plus } from "lucide-react";

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
import { Input } from "@/components/motion/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/motion/select";
import { SecretKeyPicker } from "@/components/secret-key-picker";
import { Label } from "@/components/ui/label";
import { apiFetch } from "@/lib/client-api";

type Environment = { id: string; name: string; slug: string };
type Identity = { id: string; name: string; actorType: string };

export function CreateTokenDialog({
  projectId,
  environments,
  identities,
}: {
  projectId: string;
  environments: Environment[];
  identities: Identity[];
}) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [identityId, setIdentityId] = useState("");
  const [environmentId, setEnvironmentId] = useState(environments[0]?.id ?? "");
  const [secretKeys, setSecretKeys] = useState<string[]>([]);
  const [expiresAt, setExpiresAt] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  /**
   * The minted credential, held only until the dialog closes.
   *
   * The server keeps a verifier, not the credential, so this really is the only
   * time it exists anywhere outside the holder's hands. The interface says so
   * rather than letting someone assume they can come back for it.
   */
  const [minted, setMinted] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  function close() {
    setOpen(false);
    setMinted(null);
    setCopied(false);
    setName("");
    setSecretKeys([]);
    setExpiresAt("");
    setError(null);
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError(null);

    try {
      const result = await apiFetch<{ token: string }>(
        `/api/projects/${projectId}/tokens`,
        {
          method: "POST",
          body: {
            identityId,
            name,
            environmentId,
            // A runtime token carries USE_SECRET and nothing else. Delivering a
            // value to a process is what it is for; revealing one to a person is
            // a different act on a different surface.
            capabilities: ["USE_SECRET"],
            secretKeys,
            expiresAt: expiresAt ? new Date(expiresAt).toISOString() : "",
          },
        },
      );
      setMinted(result.token);
      router.refresh();
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "Could not create this token.",
      );
    } finally {
      setPending(false);
    }
  }

  async function copy() {
    if (!minted) return;
    await navigator.clipboard.writeText(minted);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? setOpen(true) : close())}
    >
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus />
          New token
        </Button>
      </DialogTrigger>

      <DialogContent>
        {minted ? (
          <>
            <DialogHeader>
              <DialogTitle>Token created</DialogTitle>
              <DialogDescription>
                Copy it now. Only a verifier is stored, so this cannot be shown
                again, if it is lost, create a new one and revoke this.
              </DialogDescription>
            </DialogHeader>

            <div className="flex items-center gap-2 rounded-md border bg-muted px-2.5 py-2">
              <code className="min-w-0 flex-1 break-all font-mono text-meta">
                {minted}
              </code>
              <Button
                variant="ghost"
                size="icon"
                onClick={copy}
                aria-label="Copy token"
              >
                {copied ? <Check /> : <Copy />}
              </Button>
            </div>

            <p className="mt-2 text-meta text-muted-foreground">
              Give it to the runtime through its own environment, as{" "}
              <span className="font-mono">WARDER_TOKEN</span>. Do not commit it,
              and do not paste it into a shell where it will land in history.
            </p>

            <DialogFooter>
              <Button onClick={close}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>New runtime token</DialogTitle>
              <DialogDescription>
                The token is scoped to one environment and can only use secrets
                its identity has been granted.
              </DialogDescription>
            </DialogHeader>

            <form onSubmit={submit} className="space-y-4">
              <div className="space-y-1.5">
                <Input
                  id="token-name"
                  label="Name"
                  value={name}
                  onChange={setName}
                  placeholder="production deploy"
                  required
                />
              </div>

              <div className="space-y-1.5">
                <Label>Identity</Label>
                <Select value={identityId} onValueChange={setIdentityId}>
                  <SelectTrigger>
                    <SelectValue placeholder="Choose an identity…" />
                  </SelectTrigger>
                  <SelectContent>
                    {identities.map((identity) => (
                      <SelectItem key={identity.id} value={identity.id}>
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

              <div className="space-y-1.5">
                <Label>Limit to specific secrets (optional)</Label>
                <SecretKeyPicker
                  environmentId={environmentId}
                  selected={secretKeys}
                  onChange={setSecretKeys}
                />
                <p className="prose-note text-muted-foreground">
                  Choosing none lets this token reach every secret its identity
                  is granted in this environment. Narrowing it is what bounds a
                  compromised process to what it actually uses.
                </p>
              </div>

              <div className="space-y-1.5">
                <Input
                  id="token-expiry"
                  label="Expires (optional)"
                  type="datetime-local"
                  value={expiresAt}
                  onChange={setExpiresAt}
                />
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
                <Button type="submit" loading={pending}>
                  Create token
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
