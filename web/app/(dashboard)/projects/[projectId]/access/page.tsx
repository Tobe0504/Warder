import { AccessMatrix, type Grant } from "@/components/access-matrix";
import { GrantAccessDialog } from "@/components/grant-access-dialog";
import { PageTitle } from "@/components/page-shell";
import { callCoreApi } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

type Environment = { id: string; name: string; slug: string };
type Identity = { id: string; name: string; actorType: string; active: boolean };
type Member = { userId: string; name: string; email: string; role: string; active: boolean };

export default async function AccessPage({ params }: Params) {
  const { projectId } = await params;

  const [grants, environments, identities, members] = await Promise.all([
    callCoreApi<{ grants: Grant[] | null }>(`/projects/${projectId}/access`),
    callCoreApi<{ environments: Environment[] | null }>(`/projects/${projectId}/environments`),
    callCoreApi<{ identities: Identity[] | null }>("/identities"),
    callCoreApi<{ members: Member[] | null }>("/members"),
  ]);

  const environmentList = environments.environments ?? [];

  return (
    <div>
      <PageTitle
        title="Access"
        description="Who can use these secrets, and who can see them. Two different questions."
        actions={
          <GrantAccessDialog
            projectId={projectId}
            environments={environmentList}
            identities={(identities.identities ?? []).filter((i) => i.active)}
            members={(members.members ?? []).filter((m) => m.active)}
          />
        }
      />

      <AccessMatrix
        projectId={projectId}
        grants={grants.grants ?? []}
        environments={environmentList}
      />
    </div>
  );
}
