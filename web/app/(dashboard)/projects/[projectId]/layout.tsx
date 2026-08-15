import Link from "next/link";
import { notFound } from "next/navigation";

import { Crumb, CrumbSeparator, PageHeader } from "@/components/page-header";
import { ProjectNav } from "@/components/project-nav";
import { callCoreApi, CoreApiError } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

type Project = {
  id: string;
  name: string;
  slug: string;
};

/**
 * The project shell: header bar, then a nav column beside the section content.
 *
 * The nav lives here rather than in each page so that moving between Secrets,
 * Access, Tokens, and Audit re-renders only the panel: the column stays put
 * and the active item does not flicker.
 *
 * Wider than the organization-level pages, because the nav column takes a
 * chunk of the width and the tables underneath it should not pay for that.
 */
export default async function ProjectLayout({
  children,
  params,
}: { children: React.ReactNode } & Params) {
  const { projectId } = await params;

  let project: Project;
  try {
    project = await callCoreApi<Project>(`/projects/${projectId}`);
  } catch (error) {
    // A project in another organization answers 404 from the core API, and it
    // renders as "not found" here too. The interface never distinguishes
    // "someone else's" from "does not exist".
    if (error instanceof CoreApiError && error.status === 404) {
      notFound();
    }
    throw error;
  }

  return (
    <>
      <PageHeader
        breadcrumb={
          <>
            <Link
              href="/projects"
              className="rounded text-muted-foreground hover:text-foreground"
            >
              Projects
            </Link>
            <CrumbSeparator />
            <Crumb>{project.name}</Crumb>
            <span className="hidden sm:contents">
              <CrumbSeparator />
              <Crumb muted mono>
                {project.slug}
              </Crumb>
            </span>
          </>
        }
      />

      <div className="mx-auto flex w-full max-w-[1400px] flex-1 flex-col gap-4 px-4 py-4 md:flex-row md:gap-7">
        <ProjectNav projectId={projectId} />

        <main id="main" className="min-w-0 flex-1">
          {children}
        </main>
      </div>
    </>
  );
}
