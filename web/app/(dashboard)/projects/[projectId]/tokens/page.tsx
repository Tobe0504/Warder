import { CreateTokenDialog } from "@/components/create-token-dialog";
import { PageTitle } from "@/components/page-shell";
import { TokensTable, type RuntimeToken } from "@/components/tokens-table";
import { callCoreApi } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

type Environment = { id: string; name: string; slug: string };
type Identity = { id: string; name: string; actorType: string; active: boolean };

export default async function TokensPage({ params }: Params) {
  const { projectId } = await params;

  const [tokens, environments, identities] = await Promise.all([
    callCoreApi<{ tokens: RuntimeToken[] | null }>(`/projects/${projectId}/tokens`),
    callCoreApi<{ environments: Environment[] | null }>(`/projects/${projectId}/environments`),
    callCoreApi<{ identities: Identity[] | null }>("/identities"),
  ]);

  return (
    <div>
      <PageTitle
        title="Runtime tokens"
        description="Each token is bound to one identity, one project, and one environment."
        actions={
          <CreateTokenDialog
            projectId={projectId}
            environments={environments.environments ?? []}
            identities={(identities.identities ?? []).filter((i) => i.active)}
          />
        }
      />

      <TokensTable tokens={tokens.tokens ?? []} />
    </div>
  );
}
