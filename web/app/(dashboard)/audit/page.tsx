import { AuditTable, type AuditEvent } from "@/components/audit-table";
import { Crumb, PageShell, PageTitle } from "@/components/page-shell";
import { callCoreApi } from "@/lib/core-api";

export default async function OrganizationAuditPage() {
  const { events } = await callCoreApi<{ events: AuditEvent[] | null }>(
    "/audit?limit=100",
  );

  return (
    <PageShell breadcrumb={<Crumb>Audit</Crumb>}>
      <PageTitle
        title="Audit"
        description="Every security-relevant action across the organization, newest first. Secret names are recorded; values never are."
      />

      <AuditTable events={events ?? []} />

      <p className="mt-6 prose-note text-muted-foreground">
        This trail is append-only, the application cannot alter or delete an
        entry once it is written.
      </p>
    </PageShell>
  );
}
