import type { Grant } from "@/components/access-matrix";
import { cn } from "@/lib/utils";

type Environment = { id: string; name: string; slug: string };
type SecretRef = { id: string; key: string };

/**
 * Access as a grid: identities down, secrets across.
 *
 * The screen used to be a list of grants, which answers "what did we give
 * Ada". That is rarely the question. The question that brings somebody here is
 * "who can reach this key", and in a list that means reading every row and
 * holding the answer in your head. Here it is a column.
 *
 * A filled dot is software that may use the credential. A ring is a person who
 * may read it. Shape as well as colour, so the pair survives greyscale and the
 * common colour-blindness cases, where two mid-lightness hues alone would not.
 */
export function AccessGrid({
  environment,
  secrets,
  grants,
}: {
  environment: Environment;
  secrets: SecretRef[];
  grants: Grant[];
}) {
  // A grant reaches a secret when it names it, or when it is broad enough to
  // cover it: an environment grant covers every secret in that environment,
  // and a project grant covers every environment.
  const applicable = grants.filter(
    (grant) =>
      grant.environmentId === null || grant.environmentId === environment.id,
  );

  const subjects = [
    ...new Map(
      applicable.map((grant) => [
        grant.subjectId,
        {
          id: grant.subjectId,
          name: grant.subjectName,
          kind: grant.subjectKind,
          type: grant.subjectType,
        },
      ]),
    ).values(),
  ].sort((a, b) => a.name.localeCompare(b.name));

  if (secrets.length === 0 || subjects.length === 0) {
    return null;
  }

  const reaches = (subjectId: string, secretId: string) => {
    const matching = applicable.filter(
      (grant) =>
        grant.subjectId === subjectId &&
        (grant.secretId === null || grant.secretId === secretId),
    );
    return {
      use: matching.some((grant) => grant.canUse),
      read: matching.some((grant) => grant.canReveal),
    };
  };

  return (
    <section className="min-w-0">
      <h3 className="mb-2 text-meta font-medium">
        {environment.name}
        <span className="ml-2 font-normal text-muted-foreground">
          {secrets.length} {secrets.length === 1 ? "secret" : "secrets"}
        </span>
      </h3>

      {/* Scrolls rather than squeezing: a grid of forty keys is a wide thing. */}
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full min-w-max border-collapse">
          <thead>
            <tr className="border-b bg-muted/30">
              <th
                scope="col"
                className="sticky left-0 z-10 bg-card px-3 py-2 text-left text-micro font-medium uppercase tracking-wider text-muted-foreground"
              >
                Identity
              </th>
              {secrets.map((secret) => (
                <th
                  key={secret.id}
                  scope="col"
                  className="border-l px-2 py-2 text-center text-micro font-normal text-muted-foreground"
                >
                  <span className="block max-w-[7rem] truncate" title={secret.key}>
                    {secret.key}
                  </span>
                </th>
              ))}
            </tr>
          </thead>

          <tbody>
            {subjects.map((subject) => (
              <tr key={subject.id} className="border-b last:border-b-0">
                <th
                  scope="row"
                  className="sticky left-0 z-10 bg-card px-3 py-1.5 text-left font-normal"
                >
                  <span className="flex items-center gap-2">
                    {/*
                      A two-letter mark, so a long column of names still has
                      something to catch the eye at the start of each row.
                    */}
                    <span
                      aria-hidden
                      className="grid size-[18px] shrink-0 place-items-center rounded bg-muted text-[9px] font-medium text-muted-foreground"
                    >
                      {initials(subject.name, subject.kind)}
                    </span>
                    <span className="min-w-0">
                      <span className="block max-w-[11rem] truncate text-meta">
                        {subject.name}
                      </span>
                      <span className="block text-micro text-muted-foreground">
                        {subject.kind.toLowerCase().replace("_", " ")}
                      </span>
                    </span>
                  </span>
                </th>

                {secrets.map((secret) => {
                  const { use, read } = reaches(subject.id, secret.id);
                  const label = use && read
                    ? "may use and read"
                    : use
                      ? "may use"
                      : read
                        ? "may read"
                        : "no access";

                  return (
                    <td
                      key={secret.id}
                      className="border-l px-2 py-1.5 text-center"
                    >
                      <span className="sr-only">
                        {subject.name} {label} {secret.key}
                      </span>
                      <span
                        aria-hidden
                        className="inline-flex items-center justify-center gap-1"
                      >
                        {use && (
                          <span className="size-2 rounded-full bg-can-use" />
                        )}
                        {read && (
                          <span className="size-2 rounded-full border-[2px] border-can-reveal" />
                        )}
                        {!use && !read && (
                          <span className="size-[3px] rounded-full bg-border" />
                        )}
                      </span>
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

/**
 * Two letters for a row's mark: the kind for a machine, the name for a person.
 *
 * A workload called "api-runtime" is better recognised by what it is than by
 * "AP", and a person is better recognised by their initials than by "US".
 */
function initials(name: string, kind: string): string {
  const machine: Record<string, string> = {
    WORKLOAD: "WL",
    CI: "CI",
    AI_AGENT: "AI",
    SERVICE: "SV",
  };
  const mark = machine[kind.toUpperCase()];
  if (mark) return mark;

  const parts = name.trim().split(/[\s@._-]+/).filter(Boolean);
  const first = parts[0]?.[0] ?? "?";
  const second = parts[1]?.[0] ?? parts[0]?.[1] ?? "";
  return (first + second).toUpperCase();
}

/** The key to the dots, stated once rather than guessed at. */
export function AccessLegend({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-x-5 gap-y-1.5 text-meta text-muted-foreground",
        className,
      )}
    >
      <span className="inline-flex items-center gap-2">
        <span className="size-2 rounded-full bg-can-use" />
        May use it, never sees the value
      </span>
      <span className="inline-flex items-center gap-2">
        <span className="size-2 rounded-full border-[2px] border-can-reveal" />
        May read the value, and every read is recorded
      </span>
      <span className="inline-flex items-center gap-2">
        <span className="size-[3px] rounded-full bg-border" />
        No access
      </span>
    </div>
  );
}
