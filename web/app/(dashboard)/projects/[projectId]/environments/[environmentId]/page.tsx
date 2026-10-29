import { AddSecretsDialog } from "@/components/add-secrets-dialog";
import { EnvironmentSwitcher } from "@/components/environment-switcher";
import { ProjectShell } from "@/components/project-shell";
import { RunCommand } from "@/components/run-command";
import {
  SecretsTable,
  type Secret,
  type Subject,
} from "@/components/secrets-table";
import { callCoreApi } from "@/lib/core-api";
import { cliRuntimeUrl } from "@/lib/env";

type Params = {
  params: Promise<{ projectId: string; environmentId: string }>;
  searchParams: Promise<{ env?: string }>;
};

type Environment = { id: string; name: string; slug: string };
type Project = { id: string; name: string; slug: string };

/**
 * One grant, as the access API returns it.
 *
 * `secretId` and `environmentId` are null when the grant is broader than that
 * level: a grant on the project reaches every secret in it.
 */
type Grant = {
  id: string;
  subjectName: string;
  subjectKind: string;
  secretId: string | null;
  environmentId: string | null;
  canUse: boolean;
  canReveal: boolean;
  expiresAt: string | null;
};

export default async function EnvironmentSecretsPage({ params, searchParams }: Params) {
  const [{ projectId, environmentId }, { env }] = await Promise.all([params, searchParams]);

  const [project, { environments }] = await Promise.all([
    callCoreApi<Project>(`/projects/${projectId}`),
    callCoreApi<{ environments: Environment[] | null }>(
      `/projects/${projectId}/environments`,
    ),
  ]);

  // `?env=<slug>` selects the environment; the route parameter is the fallback.
  // An unknown slug falls back rather than 404ing, so a stale or hand-edited
  // link still lands somewhere useful.
  const selected =
    (env ? environments?.find((candidate) => candidate.slug === env) : undefined)?.id ??
    environmentId;

  const listing = await callCoreApi<{
    environment: Environment;
    secrets: Secret[] | null;
  }>(`/environments/${selected}/secrets`);

  const secrets = listing.secrets ?? [];

  /*
    Who can reach each of these.

    The table's two dots are the product's whole claim made visible: software
    that may use a credential, and people who may read it. That is a question
    about grants, not about the secret, so the grants come along with it.

    A failure here costs the dots, not the page: the table still lists what
    exists, which is the more important half.
  */
  let grants: Grant[] = [];
  try {
    const access = await callCoreApi<{ grants: Grant[] | null }>(
      `/projects/${projectId}/access`,
    );
    grants = access.grants ?? [];
  } catch {
    grants = [];
  }

  // A grant reaches a secret when it names it, or when it is broad enough to
  // cover it: an environment grant covers every secret in that environment,
  // and a project grant covers every environment.
  const reach = new Map<string, { use: Subject[]; read: Subject[] }>();
  for (const secret of secrets) {
    const applies = grants.filter(
      (grant) =>
        (grant.secretId === null || grant.secretId === secret.id) &&
        (grant.environmentId === null || grant.environmentId === selected),
    );
    reach.set(secret.id, {
      use: applies
        .filter((grant) => grant.canUse)
        .map((grant) => ({ name: grant.subjectName, kind: grant.subjectKind })),
      read: applies
        .filter((grant) => grant.canReveal)
        .map((grant) => ({
          name: grant.subjectName,
          kind: grant.subjectKind,
          expiresAt: grant.expiresAt,
        })),
    });
  }


  // The counts the toolbar carries. Stated at the top because the one number
  // worth noticing is how many of these a person can read, and a reader should
  // not have to total a column to find it.
  const usable = secrets.filter((s) => (reach.get(s.id)?.use.length ?? 0) > 0).length;
  const readable = secrets.filter((s) => (reach.get(s.id)?.read.length ?? 0) > 0).length;

  return (
    <ProjectShell
      project={project}
      section="Secrets"
      actions={
        <AddSecretsDialog
          environments={environments ?? []}
          currentEnvironmentId={selected}
        />
      }
    >
      {/*
        A toolbar, not a title block. The breadcrumb above already names the
        page, so this strip spends its height on the two things that change
        what you are looking at: which environment, and how much of it anyone
        can read.
      */}
      <div className="mb-4 flex flex-wrap items-center gap-x-4 gap-y-2 border-b pb-3">
        <EnvironmentSwitcher
          environments={environments ?? []}
          currentId={selected}
        />

        <div className="ml-auto flex items-center gap-4 text-meta text-muted-foreground">
          <span>
            {secrets.length} {secrets.length === 1 ? "secret" : "secrets"}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-[7px] rounded-full bg-can-use" />
            {usable} usable
          </span>
          <span
            className={
              readable > 0
                ? "inline-flex items-center gap-1.5 text-can-reveal"
                : "inline-flex items-center gap-1.5"
            }
          >
            <span
              className={
                readable > 0
                  ? "size-[7px] rounded-full border-[1.5px] border-can-reveal"
                  : "size-[7px] rounded-full border-[1.5px] border-border"
              }
            />
            {readable} readable
          </span>
        </div>
      </div>

      <SecretsTable
        secrets={secrets}
        reach={Object.fromEntries(reach)}
        projectId={projectId}
      />

      {/*
        The bridge between this screen and what the product actually does.
        Someone looking at a column of masked values has a fair question: "so
        how do I use these?", and the answer belongs on the same screen,
        already filled in with the project and environment they are looking at.
      */}
      <RunCommand
        project={project.slug}
        environment={listing.environment.slug}
        runtimeUrl={cliRuntimeUrl()}
        className="mt-5"
      />
    </ProjectShell>
  );
}
