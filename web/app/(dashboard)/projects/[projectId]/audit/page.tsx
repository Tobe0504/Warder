import { AuditTable, type AuditEvent } from "@/components/audit-table";
import { ProjectShell } from "@/components/project-shell";
import { callCoreApi } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

export default async function ProjectAuditPage({ params }: Params) {
  const { projectId } = await params;

  const [project, { events }] = await Promise.all([
    callCoreApi<{ id: string; name: string; slug: string }>(
      `/projects/${projectId}`,
    ),
    callCoreApi<{ events: AuditEvent[] | null }>(
      `/projects/${projectId}/audit?limit=100`,
    ),
  ]);

  return (
    <ProjectShell project={project} section="Audit">
      <p className="mb-4 max-w-[72ch] text-meta leading-relaxed text-muted-foreground">
        Everything that happened in this project. Names are recorded; values
        never are.
      </p>
      <AuditTable events={events ?? []} />
    </ProjectShell>
  );
}
