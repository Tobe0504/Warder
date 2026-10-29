import { AccessGrid, AccessLegend } from "@/components/access-grid";
import { AccessMatrix, type Grant } from "@/components/access-matrix";
import { GrantAccessDialog } from "@/components/grant-access-dialog";
import { ProjectShell } from "@/components/project-shell";
import { callCoreApi } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

type Environment = { id: string; name: string; slug: string };
type SecretRef = { id: string; key: string };
type Identity = {
  id: string;
  name: string;
  actorType: string;
  active: boolean;
};
type Member = {
  userId: string;
  name: string;
  email: string;
  role: string;
  active: boolean;
};

export default async function AccessPage({ params }: Params) {
  const { projectId } = await params;

  const [project, grants, environments, identities, members] =
    await Promise.all([
      callCoreApi<{ id: string; name: string; slug: string }>(
        `/projects/${projectId}`,
      ),
      callCoreApi<{ grants: Grant[] | null }>(`/projects/${projectId}/access`),
      callCoreApi<{ environments: Environment[] | null }>(
        `/projects/${projectId}/environments`,
      ),
      callCoreApi<{ identities: Identity[] | null }>("/identities"),
      callCoreApi<{ members: Member[] | null }>("/members"),
    ]);

  const environmentList = environments.environments ?? [];

  /*
    The grid's columns are this project's secrets, one grid per environment,
    because a grant is scoped to an environment and mixing them would put two
    different answers in one row.

    A failure costs the grid, not the page: the grant list below still answers
    everything, just one row at a time.
  */
  const perEnvironment = await Promise.all(
    environmentList.map(async (environment) => {
      try {
        const listing = await callCoreApi<{ secrets: SecretRef[] | null }>(
          `/environments/${environment.id}/secrets`,
        );
        return { environment, secrets: listing.secrets ?? [] };
      } catch {
        return { environment, secrets: [] as SecretRef[] };
      }
    }),
  );

  return (
    <ProjectShell
      project={project}
      section="Access"
      actions={
        <GrantAccessDialog
          projectId={projectId}
          environments={environmentList}
          identities={(identities.identities ?? []).filter((i) => i.active)}
          members={(members.members ?? []).filter((m) => m.active)}
        />
      }
    >
      <p className="mb-4 max-w-[72ch] text-meta leading-relaxed text-muted-foreground">
        A grid, because the question is never &ldquo;what can Ada do&rdquo; on
        its own, it is &ldquo;who can reach this key&rdquo;. Reading down a
        column finds everyone who can touch one credential; reading across a row
        finds everything one identity can touch.
      </p>

      {/*
        Grid first, then the grants themselves. The grid answers "who can reach
        this key", which is what people arrive with; the list below is where a
        single grant is read in full and revoked, which the grid has no room to
        do.
      */}
      <div className="space-y-5">
        {perEnvironment.some(({ secrets }) => secrets.length > 0) && (
          <div className="space-y-4">
            {perEnvironment.map(({ environment, secrets }) => (
              <AccessGrid
                key={environment.id}
                environment={environment}
                secrets={secrets}
                grants={grants.grants ?? []}
              />
            ))}
            <AccessLegend />
          </div>
        )}

        <div>
          <h2 className="mb-2 text-meta font-medium">Grants</h2>
          <AccessMatrix
            projectId={projectId}
            grants={grants.grants ?? []}
            environments={environmentList}
          />
        </div>
      </div>
    </ProjectShell>
  );
}
