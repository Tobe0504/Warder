"use client";

import { useState } from "react";
import { Check, Copy, Terminal } from "lucide-react";

import { cn } from "@/lib/utils";
import {
  ActionSwapCascadeButton,
  type ActionSwapItem,
} from "@/components/motion/action-swap-cascade";

export function RunCommand({
  project,
  environment,
  runtimeUrl,
  className,
}: {
  project: string;
  environment: string;
  /**
   * Where this Warder's runtime API lives, when the operator has published it.
   *
   * Without it the commands below say a bare `ward init` and `ward login`,
   * which point the CLI at 127.0.0.1:8081. That is right for someone running
   * Warder locally and useless for everyone else: a person who installed
   * `ward` to use a deployed Warder has no way to guess the address, and the
   * failure arrives later, from a different command, as a connection error.
   */
  runtimeUrl?: string | null;
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

  const server = runtimeUrl ? ` --url ${runtimeUrl}` : "";
  const initCommand = `ward init --project ${project} --env ${environment}${server}`;
  // The URL is recorded by `ward init`, so login only needs it when there is no
  // project file to read it from.
  const loginCommand = "ward login";

  async function copy(text: string, label: string) {
    await navigator.clipboard.writeText(text);
    setCopied(label);
    setTimeout(() => setCopied(null), 2000);
  }

  return (
    <div className={cn("rounded-xl border bg-card", className)}>
      <div className="flex items-center gap-1.5 border-b px-3 py-2">
        <Terminal className="size-3.5 text-muted-foreground" />
        <span className="text-meta font-medium">
          Use these from your application
        </span>
      </div>

      <div className="grid gap-4 p-3 md:grid-cols-3">
        <Snippet
          step={1}
          label="Point this directory at the environment"
          command={initCommand}
          hint="Writes .warder.json: safe to commit, holds no credentials."
          copied={copied === "init"}
          onCopy={() => copy(initCommand, "init")}
        />

        <Snippet
          step={2}
          label="Sign in once per machine"
          command={loginCommand}
          hint="Stores a session in your home directory, readable only by you."
          copied={copied === "login"}
          onCopy={() => copy(loginCommand, "login")}
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

const CTA_ITEMS: ActionSwapItem[] = [
  {
    id: "copy",
    label: "",
    icon: <Copy className="h-4 w-4" />,
    ariaLabel: "Copy link",
  },
  {
    id: "copied",
    label: "",
    icon: <Check className="h-4 w-4" />,
    ariaLabel: "Copied",
  },
];

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
        {/* <Button
          variant="ghost"
          size="icon"
          onClick={onCopy}
          aria-label={`Copy: ${label}`}
          className="size-6 shrink-0"
        >
          {copied ? <Check className="text-can-use" /> : <Copy />}
        </Button> */}

        <ActionSwapCascadeButton
          items={CTA_ITEMS}
          variant="primary"
          className="p-1.5 h-auto flex items-center justify-center"
          onValueChange={onCopy}
        />
      </div>
      {/* <p className="mt-1 prose-note text-muted-foreground">{hint}</p> */}
    </div>
  );
}
