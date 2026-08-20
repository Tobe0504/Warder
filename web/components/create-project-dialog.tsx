"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Plus } from "lucide-react";

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
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

export function CreateProjectDialog() {
  const router = useRouter();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  function onNameChange(value: string) {
    setName(value);
    if (!slugEdited) {
      setSlug(
        value
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, "-")
          .replace(/^-+|-+$/g, "")
          .slice(0, 63),
      );
    }
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError(null);

    try {
      const project = await apiFetch<{ id: string }>("/api/projects", {
        method: "POST",
        body: { name, slug },
      });
      setOpen(false);
      setName("");
      setSlug("");
      setSlugEdited(false);
      toast.success(`${name} created with development, staging, and production.`);
      router.push(`/projects/${project.id}`);
      router.refresh();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not create this project.");
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus />
          New project
        </Button>
      </DialogTrigger>

      <DialogContent>
        <DialogHeader>
          <DialogTitle>New project</DialogTitle>
          <DialogDescription>
            Development, staging, and production environments are created with it. You can add
            more later.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="space-y-4">
          <Input
            id="project-name"
            label="Name"
            value={name}
            onChange={onNameChange}
            placeholder="Payments API"
            required
          />

          <div className="space-y-1.5">
            <Input
              id="project-slug"
              label="Slug"
              value={slug}
              onChange={(v) => {
                setSlugEdited(true);
                setSlug(v);
              }}
              placeholder="payments-api"
              classNames={{ input: "font-mono" }}
              pattern="[a-z0-9]([a-z0-9-]*[a-z0-9])?"
              required
            />
            <p className="text-meta text-muted-foreground">
              Used by the CLI: <span className="font-mono">ward run --project {slug || "…"}</span>
            </p>
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
              Create project
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
