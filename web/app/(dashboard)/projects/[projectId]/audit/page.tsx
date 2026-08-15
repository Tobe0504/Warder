import { AuditTable, type AuditEvent } from "@/components/audit-table";
import { PageTitle } from "@/components/page-shell";
import { callCoreApi } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

export default async function ProjectAuditPage({ params }: Params) {
  const { projectId } = await params;
  const { events } = await callCoreApi<{ events: AuditEvent[] | null }>(
    `/projects/${projectId}/audit?limit=100`,
  );

  return (
    <div>
      <PageTitle
        title="Audit"
        description="Everything that happened in this project. Names are recorded; values never are."
      />
      <AuditTable events={events ?? []} />
    </div>
  );
}
