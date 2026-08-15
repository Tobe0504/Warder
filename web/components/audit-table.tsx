"use client";

import { useMemo, useState } from "react";
import {
  Bot,
  GitBranch,
  ScrollText,
  Server,
  User,
  Workflow,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import {
  DataList,
  DataListRow,
  EmptyState,
  RowMeta,
  StatusDot,
} from "@/components/ui/data-list";
import {
  FilterBar,
  FilterReset,
  FilterSearch,
  FilterSelect,
} from "@/components/ui/filter-bar";
import { RelativeTime } from "@/components/ui/relative-time";


export type AuditEvent = {
  id: string;
  occurredAt: string;
  eventType: string;
  outcome: "SUCCESS" | "DENIED" | "FAILURE";
  actorType: string;
  actor: string;
  secretKey: string;
  reason: string;
  ipAddress: string;
};

/**
 * Event types rendered as readable sentences rather than as the constants they
 * are stored as. `SECRET_REVEAL_REQUESTED` is precise and unreadable at a
 * glance; "Reveal requested" is what someone scanning for something unusual can
 * actually scan. The stored constant stays in the row's title attribute for
 * anyone matching against a query.
 */
const DESCRIPTIONS: Record<string, string> = {
  SECRET_CREATED: "Secret created",
  SECRET_ROTATED: "Secret rotated",
  SECRET_ROLLED_BACK: "Rolled back",
  SECRET_REVOKED: "Version revoked",
  SECRET_EXPIRY_CHANGED: "Expiry changed",
  SECRET_DELETED: "Secret deleted",
  SECRET_USED: "Used by a runtime",
  SECRET_REVEAL_REQUESTED: "Reveal requested",
  SECRET_REVEALED: "Value revealed",
  TOKEN_CREATED: "Token created",
  TOKEN_REVOKED: "Token revoked",
  RUNTIME_AUTHENTICATED: "Runtime signed in",
  ACCESS_GRANTED: "Access granted",
  ACCESS_REVOKED: "Access revoked",
  ACCESS_DENIED: "Access denied",
  IDENTITY_CREATED: "Identity created",
  IDENTITY_DISABLED: "Identity disabled",
  USER_INVITED: "Member added",
  USER_REMOVED: "Member removed",
  PROJECT_CREATED: "Project created",
  ENVIRONMENT_CREATED: "Environment created",
  LOGIN: "Signed in",
  LOGIN_FAILED: "Sign-in failed",
  LOGOUT: "Signed out",
  DECRYPTION_FAILED: "Decryption failed",
  RATE_LIMITED: "Rate limited",
};

/**
 * Events worth catching the eye when scanning a long list.
 *
 * Plaintext disclosure and access changes are the two things an auditor is
 * actually looking for, so they are tinted rather than left to be found by
 * reading every row.
 */
const NOTABLE = new Set([
  "SECRET_REVEALED",
  "SECRET_REVEAL_REQUESTED",
  "ACCESS_GRANTED",
]);

const OUTCOME_OPTIONS = [
  { value: "SUCCESS", label: "Success" },
  { value: "DENIED", label: "Denied" },
  { value: "FAILURE", label: "Failure" },
];

export function AuditTable({ events }: { events: AuditEvent[] }) {
  const [query, setQuery] = useState("");
  const [outcome, setOutcome] = useState("");
  const [eventType, setEventType] = useState("");

  const typeOptions = useMemo(() => {
    const present = [...new Set(events.map((event) => event.eventType))].sort();
    return present.map((value) => ({
      value,
      label: DESCRIPTIONS[value] ?? value,
    }));
  }, [events]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return events.filter((event) => {
      if (outcome && event.outcome !== outcome) return false;
      if (eventType && event.eventType !== eventType) return false;
      if (!needle) return true;
      return (
        event.actor.toLowerCase().includes(needle) ||
        event.secretKey.toLowerCase().includes(needle) ||
        (DESCRIPTIONS[event.eventType] ?? event.eventType)
          .toLowerCase()
          .includes(needle)
      );
    });
  }, [events, query, outcome, eventType]);

  if (events.length === 0) {
    return (
      <DataList>
        <EmptyState
          icon={<ScrollText className="size-5" />}
          title="Nothing recorded yet"
          description="Every security-relevant action appears here as it happens."
        />
      </DataList>
    );
  }

  const filtersActive = query !== "" || outcome !== "" || eventType !== "";

  return (
    <>
      <FilterBar>
        <FilterSearch
          value={query}
          onChange={setQuery}
          placeholder="Search events…"
        />
        <FilterSelect
          label="Actions"
          value={eventType}
          options={typeOptions}
          onChange={setEventType}
        />
        <FilterSelect
          label="Results"
          value={outcome}
          options={OUTCOME_OPTIONS}
          onChange={setOutcome}
        />
        <FilterReset
          show={filtersActive}
          onReset={() => {
            setQuery("");
            setOutcome("");
            setEventType("");
          }}
        />
        <span className="ml-auto text-meta tabular text-muted-foreground">
          {filtered.length} of {events.length}
        </span>
      </FilterBar>

      <DataList>
        {filtered.length === 0 ? (
          <EmptyState title="No events match those filters" />
        ) : (
          filtered.map((event) => <AuditRow key={event.id} event={event} />)
        )}
      </DataList>
    </>
  );
}

function AuditRow({ event }: { event: AuditEvent }) {
  const notable = NOTABLE.has(event.eventType);
  const tone =
    event.outcome === "SUCCESS"
      ? "ready"
      : event.outcome === "DENIED"
        ? "pending"
        : "error";

  return (
    <DataListRow className="gap-y-1.5 max-md:flex-wrap" title={event.eventType}>
      {/* <ActorIcon actorType={event.actorType} /> */}

      <div className="min-w-0 basis-44">
        <div
          className={
            notable
              ? "truncate text-body font-medium text-can-reveal"
              : "truncate text-body"
          }
        >
          {DESCRIPTIONS[event.eventType] ?? event.eventType}
        </div>
        <div className="mt-0.5 truncate text-meta text-muted-foreground">
          {event.actor || "—"} ·{" "}
          {event.actorType.toLowerCase().replace("_", " ")}
        </div>
      </div>

      {/*
        The secret's key, which is the whole point: a trail that says which
        credential was involved and never what it is.
      */}
      <div className="min-w-0 flex-1 basis-40">
        {event.secretKey ? (
          <span className="font-mono text-meta">{event.secretKey}</span>
        ) : (
          <span className="text-meta text-muted-foreground">—</span>
        )}
        {event.reason && (
          <div className="mt-0.5 truncate text-meta text-muted-foreground">
            {event.reason}
          </div>
        )}
      </div>

      <StatusDot tone={tone} label={event.outcome.toLowerCase()} />

      <RowMeta>
        <RelativeTime value={event.occurredAt} />
      </RowMeta>
    </DataListRow>
  );
}

export function ActorIcon({ actorType }: { actorType: string }) {
  const Icon =
    actorType === "HUMAN"
      ? User
      : actorType === "AI_AGENT"
        ? Bot
        : actorType === "CI"
          ? GitBranch
          : actorType === "WORKLOAD"
            ? Server
            : Workflow;

  return (
    <div className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-muted/50">
      <Icon className="size-3.5 text-muted-foreground" />
    </div>
  );
}

export { Badge };
