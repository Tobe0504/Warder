import { CreateIdentityDialog } from "@/components/create-identity-dialog";
import { IdentitiesTable, type Identity } from "@/components/identities-table";
import { Crumb, PageShell, PageTitle } from "@/components/page-shell";
import { callCoreApi } from "@/lib/core-api";

/**
 * Machine identities.
 *
 * These are first-class subjects, not attributes of a person: an application, a
 * CI pipeline, an AI agent session. They get their own grants and never inherit
 * the authority of whoever created them, which is the whole reason this page
 * exists separately from the members list.
 */
export default async function IdentitiesPage() {
  const { identities } = await callCoreApi<{ identities: Identity[] | null }>("/identities");

  return (
    <PageShell breadcrumb={<Crumb>Identities</Crumb>}>
      <PageTitle
        title="Identities"
        description="Applications, pipelines, and agents that can use secrets. Each is granted access on its own."
        actions={<CreateIdentityDialog />}
      />

      <IdentitiesTable identities={identities ?? []} emptyAction={<CreateIdentityDialog />} />

      <p className="mt-6 prose-note text-muted-foreground">
        An identity holds no access on its own. Grant it what it needs from a project&rsquo;s Access
        tab, then issue it a token from that project&rsquo;s Tokens tab. Revoking one stops its
        tokens and sessions immediately, and changes no secret.
      </p>
    </PageShell>
  );
}
