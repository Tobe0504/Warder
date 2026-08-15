"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Plus, Trash2 } from "lucide-react";

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
import { Input, Select, Textarea } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";
import { looksLikeDotenv, parseDotenv } from "@/lib/dotenv";
import { cn } from "@/lib/utils";

type Environment = { id: string; name: string; slug: string };

type Row = { id: number; key: string; value: string };

let nextRowId = 0;
const newRow = (key = "", value = ""): Row => ({ id: nextRowId++, key, value });

/**
 * Adds one secret or twenty.
 *
 * There is one dialog rather than a single-secret form plus a separate import
 * screen, because the two are the same act at different sizes. Configuration
 * arrives as a group — a new service needs its database URL, its API key and
 * its signing secret together — and making people repeat a four-field form
 * eight times is how keys get missed.
 *
 * Pasting a `.env` into any key field fills the rows out. That is the actual
 * shape of the task: the file already exists, on someone's laptop, and the job
 * is to get it in here and delete it.
 */
export function AddSecretsDialog({
  environments,
  currentEnvironmentId,
}: {
  environments: Environment[];
  currentEnvironmentId: string;
}) {
  const router = useRouter();
  const toast = useToast();

  const [open, setOpen] = useState(false);
  const [environmentId, setEnvironmentId] = useState(currentEnvironmentId);
  const [rows, setRows] = useState<Row[]>([newRow()]);
  const [expiresAt, setExpiresAt] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  function reset() {
    setEnvironmentId(currentEnvironmentId);
    setRows([newRow()]);
    setExpiresAt("");
    setError(null);
    setNotice(null);
  }

  /** Replaces the rows with what a paste contained, keeping anything typed. */
  function absorb(text: string, startingAt?: number) {
    const { entries, skipped } = parseDotenv(text);
    if (entries.length === 0) return false;

    setRows((current) => {
      const kept = current.filter(
        (row, index) =>
          index !== startingAt && (row.key.trim() !== "" || row.value !== ""),
      );
      const existing = new Set(kept.map((row) => row.key.trim().toUpperCase()));

      const added = entries
        .filter((entry) => !existing.has(entry.key.toUpperCase()))
        .map((entry) => newRow(entry.key, entry.value));

      // A trailing blank row, so the next key can be typed without first
      // reaching for "Add another".
      const merged = [...kept, ...added, newRow()];
      return merged;
    });

    const parts = [
      `Filled ${entries.length} ${entries.length === 1 ? "key" : "keys"}.`,
    ];
    if (skipped.length > 0) {
      // Named, not silently dropped. A line that vanished without comment is
      // how a credential goes missing and nobody notices until something 500s.
      parts.push(
        `${skipped.length} ${skipped.length === 1 ? "line" : "lines"} could not be read (${skipped
          .map((entry) => `line ${entry.line}`)
          .join(", ")}) — add ${skipped.length === 1 ? "it" : "them"} by hand.`,
      );
    }
    setNotice(parts.join(" "));
    setError(null);
    return true;
  }

  /**
   * Intercepts a paste that is really a file.
   *
   * Only on the key field. The value field stays literal, because that is
   * precisely where somebody pastes a raw credential — and a value that happens
   * to contain an equals sign, scattered across rows, would be a far worse
   * outcome than a paste that simply lands as text.
   */
  function onPasteIntoKey(
    event: React.ClipboardEvent<HTMLInputElement>,
    index: number,
  ) {
    const text = event.clipboardData.getData("text");
    if (!looksLikeDotenv(text)) return;

    event.preventDefault();
    absorb(text, index);
  }

  const filled = rows.filter((row) => row.key.trim() !== "");

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    if (filled.length === 0) {
      setError("Add at least one key.");
      return;
    }
    const missingValue = filled.find((row) => row.value === "");
    if (missingValue) {
      setError(`${missingValue.key} has no value.`);
      return;
    }

    setPending(true);
    try {
      const result = await apiFetch<{ count: number }>(
        `/api/environments/${environmentId}/secrets/batch`,
        {
          method: "POST",
          body: {
            secrets: filled.map((row) => ({
              key: row.key.trim(),
              value: row.value,
            })),
            // A datetime-local value carries no zone, so it is normalized to
            // UTC before it leaves the browser.
            expiresAt: expiresAt ? new Date(expiresAt).toISOString() : "",
          },
        },
      );

      const environment = environments.find(
        (candidate) => candidate.id === environmentId,
      );
      toast.success(
        `${result.count} ${result.count === 1 ? "secret" : "secrets"} added to ${
          environment?.name ?? "the environment"
        } and encrypted.`,
      );
      reset();
      setOpen(false);
      router.refresh();
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "Could not add these secrets.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        // Values never survive the dialog closing, on any path.
        if (!next) reset();
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus />
          Add secrets
        </Button>
      </DialogTrigger>

      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Add secrets</DialogTitle>
          <DialogDescription>
            Values are encrypted before they are stored and will not be shown
            again unless someone is explicitly granted permission to reveal
            them.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="batch-environment">Environment</Label>
            <Select
              id="batch-environment"
              value={environmentId}
              onChange={(e) => setEnvironmentId(e.target.value)}
            >
              {environments.map((environment) => (
                <option key={environment.id} value={environment.id}>
                  {environment.name}
                </option>
              ))}
            </Select>
            <p className="prose-note text-muted-foreground">
              Everything below goes here. Production values belong in
              production, a secret added to the wrong environment is one an
              application will happily use.
            </p>
          </div>

          <div className="space-y-1.5">
            <div className="flex items-baseline justify-between gap-2">
              <Label>Secrets</Label>
              <span className="text-meta text-muted-foreground">
                Paste a .env into a key field
              </span>
            </div>

            <div className="space-y-1.5">
              {rows.map((row, index) => (
                <div key={row.id} className="flex items-start gap-1.5">
                  <Input
                    value={row.key}
                    onChange={(e) =>
                      setRows((current) =>
                        current.map((candidate) =>
                          candidate.id === row.id
                            ? {
                                ...candidate,
                                key: e.target.value.toUpperCase(),
                              }
                            : candidate,
                        ),
                      )
                    }
                    onPaste={(e) => onPasteIntoKey(e, index)}
                    placeholder="DATABASE_URL"
                    className="basis-2/5 font-mono"
                    autoComplete="off"
                    spellCheck={false}
                    aria-label={`Key ${index + 1}`}
                  />
                  {/*
                    A value containing newlines gets a textarea, not an input.
                    A single-line input silently strips line breaks: the PEM
                    key pasted a moment ago would display as one run-on string
                    and, the instant anyone touched the field, would lose its
                    line breaks for good — storing a private key that cannot be
                    used and does not look broken.
                  */}
                  {row.value.includes("\n") ? (
                    <Textarea
                      value={row.value}
                      onChange={(e) =>
                        setRows((current) =>
                          current.map((candidate) =>
                            candidate.id === row.id
                              ? { ...candidate, value: e.target.value }
                              : candidate,
                          ),
                        )
                      }
                      className="min-h-9 flex-1 font-mono text-meta"
                      rows={3}
                      autoComplete="off"
                      spellCheck={false}
                      aria-label={`Value ${index + 1}`}
                    />
                  ) : (
                    <Input
                      value={row.value}
                      onChange={(e) =>
                        setRows((current) =>
                          current.map((candidate) =>
                            candidate.id === row.id
                              ? { ...candidate, value: e.target.value }
                              : candidate,
                          ),
                        )
                      }
                      placeholder="postgres://…"
                      className="font-mono"
                      autoComplete="off"
                      spellCheck={false}
                      aria-label={`Value ${index + 1}`}
                    />
                  )}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className={cn(
                      "shrink-0",
                      rows.length === 1 && (!row.value || row.key) && "hidden",
                    )}
                    onClick={() =>
                      setRows((current) =>
                        current.filter((candidate) => candidate.id !== row.id),
                      )
                    }
                    aria-label={`Remove row ${index + 1}`}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))}
            </div>

            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-7"
              onClick={() => setRows((current) => [...current, newRow()])}
            >
              <Plus />
              Add another
            </Button>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="batch-expiry">Expires (optional)</Label>
            <Input
              id="batch-expiry"
              type="datetime-local"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
            />
            <p className="prose-note text-muted-foreground">
              Applies to every secret above.
            </p>
          </div>

          {notice && (
            <p className="text-meta text-muted-foreground">{notice}</p>
          )}

          {error && (
            <p role="alert" className="text-meta text-destructive">
              {error}
            </p>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setOpen(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={pending}>
              {pending
                ? "Saving…"
                : filled.length > 1
                  ? `Add ${filled.length} secrets`
                  : "Add secret"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
