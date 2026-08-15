"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Copy, Eye, KeyRound, MoreHorizontal, RotateCw } from "lucide-react";

import { RotateSecretDialog } from "@/components/rotate-secret-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DataList,
  DataListRow,
  EmptyState,
  MonoTag,
  RowMeta,
  StatusDot,
} from "@/components/ui/data-list";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  FilterBar,
  FilterReset,
  FilterSearch,
  FilterSelect,
} from "@/components/ui/filter-bar";
import { RelativeTime } from "@/components/ui/relative-time";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

export type Secret = {
  id: string;
  key: string;
  description: string;
  masked: string;
  version?: number;
  status: string;
  expiresAt: string | null;
  lastUsedAt: string | null;
  canUse: boolean;
  canReveal: boolean;
  canRotate: boolean;
};

/** How long a revealed value stays on screen before it clears itself. */
const REVEAL_SECONDS = 30;

const STATUS_OPTIONS = [
  { value: "ACTIVE", label: "Active" },
  { value: "EXPIRED", label: "Expired" },
  { value: "REVOKED", label: "Revoked" },
  { value: "NO_VERSION", label: "No version" },
];

export function SecretsTable({ secrets }: { secrets: Secret[] }) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return secrets.filter((secret) => {
      if (status && secret.status !== status) return false;
      if (!needle) return true;
      return (
        secret.key.toLowerCase().includes(needle) ||
        secret.description.toLowerCase().includes(needle)
      );
    });
  }, [secrets, query, status]);

  if (secrets.length === 0) {
    return (
      <DataList>
        <EmptyState
          icon={<KeyRound className="size-5" />}
          title="No secrets in this environment"
          description="Add one, then grant an application the right to use it. Nothing can reach a secret until something is explicitly granted USE_SECRET."
        />
      </DataList>
    );
  }

  const filtersActive = query !== "" || status !== "";

  return (
    <>
      <FilterBar>
        <FilterSearch
          value={query}
          onChange={setQuery}
          placeholder="Search secrets…"
        />
        <FilterSelect
          label="Statuses"
          value={status}
          options={STATUS_OPTIONS}
          onChange={setStatus}
        />
        <FilterReset
          show={filtersActive}
          onReset={() => {
            setQuery("");
            setStatus("");
          }}
        />
        <span className="ml-auto text-meta tabular text-muted-foreground">
          {filtered.length} of {secrets.length}
        </span>
      </FilterBar>

      <DataList>
        {filtered.length === 0 ? (
          <EmptyState title="No secrets match those filters" />
        ) : (
          filtered.map((secret) => (
            <SecretRow key={secret.id} secret={secret} />
          ))
        )}
      </DataList>
    </>
  );
}

function SecretRow({ secret }: { secret: Secret }) {
  const router = useRouter();
  const toast = useToast();
  const [revealed, setRevealed] = useState<string | null>(null);
  const [countdown, setCountdown] = useState(0);
  const [pending, setPending] = useState(false);
  const timers = useRef<ReturnType<typeof setInterval>[]>([]);

  /**
   * A revealed value is held in one local variable and nowhere else — not in a
   * store, not in a context, not in the URL, not in storage — and it clears
   * itself after a short window. That does not make the disclosure reversible;
   * the person saw it, and the audit trail records that. It limits how long the
   * value sits in a tab someone walked away from.
   */
  useEffect(() => () => timers.current.forEach(clearInterval), []);

  function startCountdown() {
    setCountdown(REVEAL_SECONDS);
    const interval = setInterval(() => {
      setCountdown((remaining) => {
        if (remaining <= 1) {
          clearInterval(interval);
          setRevealed(null);
          return 0;
        }
        return remaining - 1;
      });
    }, 1000);
    timers.current.push(interval);
  }

  async function reveal() {
    setPending(true);
    try {
      const result = await apiFetch<{ value: string }>(
        `/api/secrets/${secret.id}/reveal`,
        {
          method: "POST",
        },
      );
      setRevealed(result.value);
      startCountdown();
      // Said out loud, because a reveal is an audited event and the person
      // doing it should know that rather than discover it later.
      toast.success(
        `${secret.key} revealed. This was recorded in the audit trail.`,
      );
    } catch (caught) {
      toast.error(
        caught instanceof Error
          ? caught.message
          : "Could not reveal this value.",
      );
    } finally {
      setPending(false);
    }
  }

  const expired = secret.status === "EXPIRED";
  const revoked = secret.status === "REVOKED";
  const tone =
    expired || revoked
      ? "error"
      : secret.status === "ACTIVE"
        ? "ready"
        : "neutral";

  return (
    <DataListRow className="gap-y-2 max-md:flex-wrap">
      {/* <div className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-muted/50">
        <KeyRound className="size-3.5 text-muted-foreground" />
      </div> */}

      <div className="min-w-0 basis-44">
        <div className="truncate font-sans text-meta">{secret.key}</div>
        {secret.description && (
          <div className="truncate text-meta text-muted-foreground">
            {secret.description}
          </div>
        )}
      </div>

      <div className="min-w-0 flex-1 basis-22">
        {revealed ? (
          <div className="flex items-start gap-1.5">
            <span className="min-w-0 flex-1 break-all rounded bg-can-reveal-surface px-1.5 py-0.5 font-mono text-meta text-can-reveal">
              {revealed}
            </span>
            <Button
              variant="ghost"
              size="icon"
              className="size-6 shrink-0"
              aria-label={`Copy ${secret.key}`}
              onClick={() => navigator.clipboard.writeText(revealed)}
            >
              <Copy />
            </Button>
            <span
              className="shrink-0 pt-0.5 text-meta tabular text-muted-foreground"
              title="This value clears itself shortly"
            >
              {countdown}s
            </span>
          </div>
        ) : (
          // select-none so a drag-select of the row does not put a string of
          // dots on the clipboard that looks like it might be the value.
          <span className="select-none font-mono text-body text-muted-foreground">
            {secret.masked}
          </span>
        )}
      </div>

      <div className="flex shrink-0 items-center gap-3">
        <StatusDot
          tone={tone}
          label={secret.status.toLowerCase().replace("_", " ")}
        />
        {secret.version && (
          <Badge variant="outline" className="font-mono">
            v{secret.version}
          </Badge>
        )}
      </div>

      <RowMeta className="max-md:basis-full max-md:justify-end text-meta">
        <RelativeTime
          value={secret.expiresAt}
          prefix="Expires"
          fallback="No expiry"
        />
        <RelativeTime
          className="hidden md:inline"
          value={secret.lastUsedAt}
          prefix="Used"
          fallback="Never used"
        />

        <div className="flex items-center gap-1">
          {/*
            The reveal control appears only when the policy engine says this
            person holds READ_SECRET. The server checks again regardless; this
            just avoids offering a button that would be refused.
          */}
          {secret.canReveal && !revealed && (
            <Button
              variant="ghost"
              size="sm"
              onClick={reveal}
              disabled={pending}
              className="h-7 gap-1.5 px-2"
            >
              <Eye />
              <span className="text-meta">
                {pending ? "Revealing…" : "Reveal"}
              </span>
            </Button>
          )}

          {secret.canRotate && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  aria-label={`Actions for ${secret.key}`}
                >
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <RotateSecretDialog
                  secretId={secret.id}
                  secretKey={secret.key}
                  onRotated={() => router.refresh()}
                >
                  <DropdownMenuItem
                    onSelect={(event) => event.preventDefault()}
                  >
                    <RotateCw />
                    Rotate
                  </DropdownMenuItem>
                </RotateSecretDialog>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </RowMeta>
    </DataListRow>
  );
}

export { MonoTag };
