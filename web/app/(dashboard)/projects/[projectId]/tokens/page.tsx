import { CreateTokenDialog } from "@/components/create-token-dialog";
import { ProjectShell } from "@/components/project-shell";
import { TokensTable, type RuntimeToken } from "@/components/tokens-table";
import { callCoreApi } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

type Environment = { id: string; name: string; slug: string };
type Identity = { id: string; name: string; actorType: string; active: boolean };

export default async function TokensPage({ params }: Params) {
  const { projectId } = await params;

  const [project, tokens, environments, identities] = await Promise.all([
    callCoreApi<{ id: string; name: string; slug: string }>(
      `/projects/${projectId}`,
    ),
    callCoreApi<{ tokens: RuntimeToken[] | null }>(`/projects/${projectId}/tokens`),
    callCoreApi<{ environments: Environment[] | null }>(`/projects/${projectId}/environments`),
    callCoreApi<{ identities: Identity[] | null }>("/identities"),
  ]);

  return (
    <ProjectShell
      project={project}
      section="Tokens"
      actions={
        <CreateTokenDialog
          projectId={projectId}
          environments={environments.environments ?? []}
          identities={(identities.identities ?? []).filter((i) => i.active)}
        />
      }
    >
      <p className="mb-4 max-w-[72ch] text-meta leading-relaxed text-muted-foreground">
        Each token is bound to one identity, one project, and one environment.
        A stolen token reaches exactly that much and no further.
      </p>

      <TokensTable tokens={tokens.tokens ?? []} />
    </ProjectShell>
  );
}
