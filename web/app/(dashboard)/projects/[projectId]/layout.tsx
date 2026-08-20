import Link from "next/link";
import { notFound } from "next/navigation";

import { Crumb, CrumbSeparator, PageHeader } from "@/components/page-header";
import { ProjectDock } from "@/components/project-dock";
import { callCoreApi, CoreApiError } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

type Project = {
  id: string;
  name: string;
  slug: string;
};

/**
 * The project shell: header bar, then the section content at full width.
 *
 * The nav lives here rather than in each page so that moving between Secrets,
 * Access, Tokens, and Audit re-renders only the panel and the active item does
 * not flicker. It is a dock rather than a second column: the organization
 * sidebar already owns a rail, and two of them beside a secrets table left the
 * rows wrapping into three lines each.
 *
 * The bottom padding is what keeps the last row of a table clear of the dock.
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
              <Crumb muted>{project.slug}</Crumb>
            </span>
          </>
        }
      />

      <div className="mx-auto w-full max-w-[1400px] flex-1 px-4 py-4 pb-24">
        <main id="main" className="min-w-0">
          {children}
        </main>
      </div>

      <ProjectDock projectId={projectId} />
    </>
  );
}
