import { redirect } from "next/navigation";

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
    return (
      <p className="py-12 text-center text-meta text-muted-foreground">
        This project has no environments yet.
      </p>
    );
  }

  redirect(`/projects/${projectId}/environments/${target.id}`);
}
