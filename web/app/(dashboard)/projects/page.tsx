import Link from "next/link";
import { ChevronRight, FolderPlus } from "lucide-react";

import { CreateProjectDialog } from "@/components/create-project-dialog";
import { Crumb, PageShell, PageTitle } from "@/components/page-shell";
import { DataList, EmptyState } from "@/components/ui/data-list";
import { RelativeTime } from "@/components/ui/relative-time";
import { callCoreApi } from "@/lib/core-api";

type Project = {
  id: string;
  name: string;
  slug: string;
  createdAt: string;
};

export default async function ProjectsPage() {
  const { projects } = await callCoreApi<{ projects: Project[] | null }>(
    "/projects",
  );
  const list = projects ?? [];

  return (
    <PageShell breadcrumb={<Crumb>Projects</Crumb>}>
      <PageTitle
        title="Projects"
        description="Each project holds its own environments, secrets, and access."
        actions={<CreateProjectDialog />}
      />

      {list.length === 0 ? (
        <DataList>
          <EmptyState
            icon={<FolderPlus className="size-5" />}
            title="No projects yet"
            description="A project groups the environments an application runs in. Create one to store its first secret."
            action={<CreateProjectDialog />}
          />
        </DataList>
      ) : (
        /*
          Denser than a card grid. A project carries three facts, and a card
          per project spent a quarter of the screen on each of them; this is
          the same information in a row you can scan a dozen of at once.
        */
        <ul className="divide-y rounded-lg border">
          {list.map((project) => (
            <li key={project.id}>
              <Link
                href={`/projects/${project.id}`}
                className="group flex items-center gap-3 px-3 py-2.5 transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
              >
                {/* User-supplied text, rendered by React and therefore escaped. */}
                <span className="min-w-0 flex-1 truncate text-body font-medium">
                  {project.name}
                </span>

                <span className="hidden min-w-0 shrink-0 truncate text-meta text-muted-foreground sm:block">
                  {project.slug}
                </span>

                <span className="hidden shrink-0 text-meta text-muted-foreground md:block">
                  <RelativeTime value={project.createdAt} prefix="Created" />
                </span>

                <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </PageShell>
  );
}
