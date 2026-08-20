import { AddSecretsDialog } from "@/components/add-secrets-dialog";
import { EnvironmentSwitcher } from "@/components/environment-switcher";
import { PageTitle } from "@/components/page-shell";
import { RunCommand } from "@/components/run-command";
import { SecretsTable, type Secret } from "@/components/secrets-table";
import { callCoreApi } from "@/lib/core-api";

type Params = {
  params: Promise<{ projectId: string; environmentId: string }>;
  searchParams: Promise<{ env?: string }>;
};

type Environment = { id: string; name: string; slug: string };
type Project = { id: string; name: string; slug: string };

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

  return (
    <div className="space-y-6">
      <PageTitle
        title="Secrets"
        description="Values are never sent to this page. A runtime authorized with USE_SECRET receives them directly."
        actions={
          <AddSecretsDialog
            environments={environments ?? []}
            currentEnvironmentId={selected}
          />
        }
      />

      <EnvironmentSwitcher environments={environments ?? []} currentId={selected} />

      {/*
        The table gets the full width, as Vercel's deployments table does. A
        secret row carries six facts, and squeezing it beside a panel makes
        every row wrap into three lines.
      */}
      <SecretsTable secrets={secrets} />

      {/*
        The bridge between this screen and what the product actually does.
        Someone looking at a column of masked values has a fair question: "so
        how do I use these?", and the answer belongs on the same screen,
        already filled in with the project and environment they are looking at.
      */}
      <RunCommand
        project={project.slug}
        environment={listing.environment.slug}
      />
    </div>
  );
}
