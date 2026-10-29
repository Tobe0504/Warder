import { redirect } from "next/navigation";

import { ProjectShell } from "@/components/project-shell";
import { callCoreApi } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

type Environment = { id: string; slug: string };

/**
 * The project's landing page opens on an environment's secrets, since that is
 * what someone came here to look at. Development is preferred over whatever
 * happens to be first, so the default view is the least consequential one.
 */
export default async function ProjectPage({ params }: Params) {
  const { projectId } = await params;
  const { environments } = await callCoreApi<{ environments: Environment[] | null }>(
    `/projects/${projectId}/environments`,
  );

  const list = environments ?? [];
  const target = list.find((e) => e.slug === "development") ?? list[0];

  if (!target) {
    // Rare, but it must not be a bare sentence floating in an unframed page:
    // the layout stopped drawing a header, so without a shell this route has
    // no breadcrumb, no measure and no way back.
    const project = await callCoreApi<{
      id: string;
      name: string;
      slug: string;
    }>(`/projects/${projectId}`);

    return (
      <ProjectShell project={project}>
        <p className="py-10 text-center text-meta text-muted-foreground">
          This project has no environments yet.
        </p>
      </ProjectShell>
    );
  }

  redirect(`/projects/${projectId}/environments/${target.id}`);
}
