"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  CalendarClock,
  CircleOff,
  Copy,
  Eye,
  KeyRound,
  MoreHorizontal,
  RotateCw,
} from "lucide-react";

import { ChangeExpiryDialog } from "@/components/change-expiry-dialog";
import { RotateSecretDialog } from "@/components/rotate-secret-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/motion/button";
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
import { cn } from "@/lib/utils";

/** One identity a grant names. */
export type Subject = { name: string; kind: string; expiresAt?: string | null };

/** Who can reach a secret, keyed by secret id. */
export type Reach = Record<string, { use: Subject[]; read: Subject[] }>;

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

export function SecretsTable({
  secrets,
  reach = {},
  projectId,
}: {
  secrets: Secret[];
  reach?: Reach;
  projectId?: string;
}) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(
    secrets[0]?.id ?? null,
  );

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

  // The selection follows the filter: a pane describing a row that is no
  // longer on screen is a pane describing nothing the reader can see.
  const selected =
    filtered.find((secret) => secret.id === selectedId) ?? filtered[0] ?? null;

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

      {/*
        List and detail, side by side. Inspecting a secret used to mean leaving
        the list, and the question people actually arrive with, who can reach
        this, was two screens away. The pane answers it in place.
      */}
      <div className="flex flex-wrap items-start gap-4">
        <div className="min-w-0 flex-[999_1_28rem]">
          <DataList>
            {filtered.length > 0 && (
              <div className="flex items-center gap-6 border-b bg-muted/30 px-3.5 py-1.5 text-micro font-medium uppercase tracking-wider text-muted-foreground">
                <span className="min-w-[8rem] flex-[2] basis-44">Key</span>
                <span className="w-[4.5rem] shrink-0">Access</span>
                <span className="flex shrink-0 items-center gap-3">
                  <span className="w-14">Status</span>
                  <span className="w-8 text-right">Ver</span>
                </span>
                <span className="ml-auto hidden 2xl:block w-20 text-right">Used</span>
              </div>
            )}
            {filtered.length === 0 ? (
              <EmptyState
                title="No secrets match those filters"
                description="Either there are no secrets for this environment or try a different search keyword"
                icon={<CircleOff />}
              />
            ) : (
              filtered.map((secret) => (
                <SecretRow
                  key={secret.id}
                  secret={secret}
                  reach={reach[secret.id]}
                  selected={secret.id === selectedId}
                  onSelect={() => setSelectedId(secret.id)}
                />
              ))
            )}
          </DataList>
        </div>

        {selected && (
          <SecretDetail
            secret={selected}
            reach={reach[selected.id]}
            projectId={projectId}
          />
        )}
      </div>
    </>
  );
}

/**
 * The two dots.
 *
 * A filled dot is software that may use the credential; a ring is a person who
 * may read it. Shape as well as colour, so the pair survives greyscale and the
 * common colour-blindness cases, where green and amber alone would not.
 *
 * A ring should be rare. A column of them means the separation this product
 * exists to keep has stopped being kept, and that is worth seeing from the
 * list rather than three screens in.
 */
function ReachDots({ reach }: { reach?: { use: Subject[]; read: Subject[] } }) {
  const use = reach?.use.length ?? 0;
  const read = reach?.read.length ?? 0;

  return (
    <span className="flex shrink-0 items-center gap-1.5">
      <span
        title={use === 1 ? "1 identity may use this" : `${use} identities may use this`}
        className={cn(
          "size-[7px] rounded-full",
          use > 0 ? "bg-can-use" : "bg-border",
        )}
      />
      <span
        title={read === 1 ? "1 identity may read this" : `${read} identities may read this`}
        className={cn(
          "size-[7px] rounded-full border-[1.5px]",
          read > 0 ? "border-can-reveal" : "border-border",
        )}
      />
      {read > 0 && (
        <span className="text-micro tabular text-can-reveal">{read}</span>
      )}
    </span>
  );
}

function SecretRow({
  secret,
  reach,
  selected,
  onSelect,
}: {
  secret: Secret;
  reach?: { use: Subject[]; read: Subject[] };
  selected?: boolean;
  onSelect?: () => void;
}) {
  const router = useRouter();
  const toast = useToast();
  const [revealed, setRevealed] = useState<string | null>(null);
  const [countdown, setCountdown] = useState(0);
  const [pending, setPending] = useState(false);
  const timers = useRef<ReturnType<typeof setInterval>[]>([]);

  /**
   * A revealed value is held in one local variable and nowhere else: not in a
   * store, not in a context, not in the URL, not in storage, and it clears
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
    <DataListRow
      className={cn(
        "cursor-default gap-y-2 max-md:flex-wrap",
        selected && "bg-muted/40 shadow-[inset_2px_0_0_var(--foreground)]",
      )}
      onClick={onSelect}
    >
      {/* <div className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-muted/50">
        <KeyRound className="size-3.5 text-muted-foreground" />
      </div> */}

      {/*
        flex-[2] and a real minimum. This column had `basis-44` and nothing to
        defend it, so the metadata on the right held its full width while the
        secret's name, the one thing the row is about, collapsed to "DA..".
        The name now takes space before anything else does.
      */}
      <div className="min-w-[8rem] flex-[2] basis-44">
        <div className="truncate font-sans text-meta font-medium">{secret.key}</div>
        {secret.description && (
          <div className="truncate text-meta text-muted-foreground">
            {secret.description}
          </div>
        )}
      </div>

      <ReachDots reach={reach} />

      <div className={cn("min-w-0", revealed ? "flex-1 basis-20" : "hidden")}>
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
        ) : null}
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
          className="hidden xl:inline"
          value={secret.expiresAt}
          prefix="Expires"
          fallback="No expiry"
        />
        <RelativeTime
          className="hidden 2xl:inline"
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
              loading={pending}
              className="h-7 gap-1.5 px-2"
            >
              <Eye />
              <span className="text-meta">
                Reveal
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

                {/*
                  Separate from rotation on purpose. Rotating replaces the
                  value and every runtime has to pick the new one up; changing
                  the expiry leaves the credential alone. An expired date is
                  usually the only thing wrong.
                */}
                <ChangeExpiryDialog
                  secretId={secret.id}
                  secretKey={secret.key}
                  currentExpiry={secret.expiresAt}
                  onChanged={() => router.refresh()}
                >
                  <DropdownMenuItem
                    onSelect={(event) => event.preventDefault()}
                  >
                    <CalendarClock />
                    Change expiry
                  </DropdownMenuItem>
                </ChangeExpiryDialog>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </RowMeta>
    </DataListRow>
  );
}

export { MonoTag };

/**
 * The detail pane.
 *
 * It answers one question, which is the question the product exists for: who
 * can reach this credential, and in which of the two senses. Everything else
 * on screen lists what exists; this says what that means.
 */
function SecretDetail({
  secret,
  reach,
  projectId,
}: {
  secret: Secret;
  reach?: { use: Subject[]; read: Subject[] };
  projectId?: string;
}) {
  const use = reach?.use ?? [];
  const read = reach?.read ?? [];

  return (
    <aside className="min-w-0 flex-[1_1_18rem] rounded-lg border bg-card p-3.5 lg:max-w-[22rem]">
      <div className="truncate text-body font-medium">{secret.key}</div>
      <div className="mt-1 text-meta text-muted-foreground">
        {secret.version && <span>v{secret.version}</span>}
        <span aria-hidden className="px-1.5 text-border">/</span>
        <span>{secret.status.toLowerCase().replace("_", " ")}</span>
        <span aria-hidden className="px-1.5 text-border">/</span>
        <RelativeTime
          value={secret.expiresAt}
          prefix="expires"
          fallback="no expiry"
        />
      </div>

      {secret.description && (
        <p className="mt-2 text-meta leading-relaxed text-muted-foreground">
          {secret.description}
        </p>
      )}

      {/*
        Said plainly, where the question gets asked. Somebody looking at a row
        of dots wants to know where the value is; the honest answer is that it
        is not here, and that asking for it is an event rather than a glance.
      */}
      <div className="mt-3 rounded-md border bg-background/40 p-2.5">
        <p className="text-meta leading-relaxed text-muted-foreground">
          The value is not on this page. A runtime holding{" "}
          <span className="text-can-use">USE_SECRET</span> receives it directly.
        </p>
        {secret.canReveal && (
          <Button
            variant="outline"
            size="sm"
            className="mt-2.5 w-full border-can-reveal/50 text-can-reveal hover:bg-can-reveal-surface"
          >
            Reveal, and record it
          </Button>
        )}
      </div>

      <div className="mt-4">
        <div className="flex items-center gap-1.5 text-micro font-medium uppercase tracking-wider text-muted-foreground">
          <span className="size-[6px] rounded-full bg-can-use" />
          Can use
        </div>
        {use.length === 0 ? (
          <p className="mt-1.5 text-meta text-muted-foreground">
            Nothing can use this yet. A secret nothing is granted is a secret no
            application can start with.
          </p>
        ) : (
          <ul className="mt-1">
            {use.map((subject) => (
              <li
                key={`use-${subject.name}`}
                className="flex items-center gap-2 border-b py-1.5 last:border-b-0"
              >
                <span className="min-w-0 flex-1 truncate text-meta">
                  {subject.name}
                </span>
                <span className="shrink-0 text-micro text-muted-foreground">
                  {subject.kind.toLowerCase().replace("_", " ")}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="mt-4">
        <div className="flex items-center gap-1.5 text-micro font-medium uppercase tracking-wider text-can-reveal">
          <span className="size-[6px] rounded-full border-[1.5px] border-can-reveal" />
          Can read
        </div>
        {read.length === 0 ? (
          <p className="mt-1.5 text-meta text-muted-foreground">
            Nobody. The value reaches runtimes and no one else.
          </p>
        ) : (
          <ul className="mt-1">
            {read.map((subject) => (
              <li
                key={`read-${subject.name}`}
                className="flex items-center gap-2 border-b py-1.5 last:border-b-0"
              >
                <span className="min-w-0 flex-1 truncate text-meta">
                  {subject.name}
                </span>
                <RelativeTime
                  className="shrink-0 text-micro text-muted-foreground"
                  value={subject.expiresAt ?? null}
                  prefix="until"
                  fallback="no expiry"
                />
              </li>
            ))}
          </ul>
        )}
      </div>

      {projectId && (
        <Button asChild variant="outline" size="sm" className="mt-4 w-full">
          <Link href={`/projects/${projectId}/access`}>Change access</Link>
        </Button>
      )}
    </aside>
  );
}
