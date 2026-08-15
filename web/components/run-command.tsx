"use client";

import { useState } from "react";
import { Check, Copy, Terminal } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * The command to run against this environment.
 *
 * This is the piece that connects the dashboard to what the product actually
 * does. Someone looking at a list of masked values has a reasonable question:
 * "so how do I use these?", and the answer should be on the same screen,
 * already filled in with the project and environment they are looking at,
 * rather than in documentation they have to go find.
 *
 * Nothing here is secret. It is a project slug and an environment slug, both of
 * which are already on the page. The credential comes from `ward login` or
 * from the runtime's own environment, and never appears in a command line:
 * which is why the snippet shows `ward login` as a separate step rather than
 * offering a flag that would put a token into shell history.
 */
export function RunCommand({
  project,
  environment,
  className,
}: {
  project: string;
  environment: string;
  className?: string;
}) {
  const [copied, setCopied] = useState<string | null>(null);

  // The environment flag is omitted for development, since it is the default a
  // committed .warder.json would carry. Showing the shortest command that works
  // is what makes the tool feel light.
  const runCommand =
    environment === "development"
      ? `ward run -- npm run dev`
      : `ward run --env ${environment} -- npm run dev`;

  async function copy(text: string, label: string) {
    await navigator.clipboard.writeText(text);
    setCopied(label);
    setTimeout(() => setCopied(null), 2000);
  }

  return (
    <div className={cn("rounded-xl border bg-card", className)}>
      <div className="flex items-center gap-1.5 border-b px-3 py-2">
        <Terminal className="size-3.5 text-muted-foreground" />
        <span className="text-meta font-medium">Use these from your application</span>
      </div>

      {/*
        Three steps, side by side on a wide screen and stacked on a narrow one.
        They are numbered because the order matters, `ward run` before
        `ward login` fails with an error that does not explain itself.
      */}
      <div className="grid gap-4 p-3 md:grid-cols-3">
        <Snippet
          step={1}
          label="Point this directory at the environment"
          command={`ward init --project ${project} --env ${environment}`}
          hint="Writes .warder.json: safe to commit, holds no credentials."
          copied={copied === "init"}
          onCopy={() => copy(`ward init --project ${project} --env ${environment}`, "init")}
        />

        <Snippet
          step={2}
          label="Sign in once per machine"
          command="ward login"
          hint="Stores a session in your home directory, readable only by you."
          copied={copied === "login"}
          onCopy={() => copy("ward login", "login")}
        />

        <Snippet
          step={3}
          label="Run anything with the secrets injected"
          command={runCommand}
          hint="Values reach the process, not your terminal and not a file."
          copied={copied === "run"}
          onCopy={() => copy(runCommand, "run")}
          emphasis
        />
      </div>
    </div>
  );
}

function Snippet({
  step,
  label,
  command,
  hint,
  copied,
  onCopy,
  emphasis,
}: {
  step: number;
  label: string;
  command: string;
  hint: string;
  copied: boolean;
  onCopy: () => void;
  emphasis?: boolean;
}) {
  return (
    <div className="min-w-0">
      <div className="mb-1.5 flex items-start gap-1.5 text-meta text-muted-foreground">
        <span className="mt-px flex size-4 shrink-0 items-center justify-center rounded-full border text-micro tabular">
          {step}
        </span>
        <span className="leading-snug">{label}</span>
      </div>
      <div
        className={cn(
          "group flex items-start gap-2 rounded-md border px-2.5 py-1.5",
          emphasis ? "border-foreground/20 bg-muted" : "bg-transparent",
        )}
      >
        {/*
          Commands wrap rather than scroll. A command clipped at the panel edge
          reads as complete when it is not, and someone retyping from what they
          can see gets a broken command, which is a worse outcome than two
          lines of text.
        */}
        <code className="min-w-0 flex-1 whitespace-pre-wrap break-all font-mono prose-note">
          {command}
        </code>
        <Button
          variant="ghost"
          size="icon"
          onClick={onCopy}
          aria-label={`Copy: ${label}`}
          className="size-6 shrink-0"
        >
          {copied ? <Check className="text-can-use" /> : <Copy />}
        </Button>
      </div>
      <p className="mt-1 prose-note text-muted-foreground">{hint}</p>
    </div>
  );
}
