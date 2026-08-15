import Link from "next/link";
import { ChevronRight, FolderPlus } from "lucide-react";

import { CreateProjectDialog } from "@/components/create-project-dialog";
import { Crumb, PageShell, PageTitle } from "@/components/page-shell";
import {
  DataList,
  DataListRow,
  EmptyState,
  MonoTag,
  RowMeta,
} from "@/components/ui/data-list";
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
        <DataList>
          {list.map((project) => (
            <Link
              key={project.id}
              href={`/projects/${project.id}`}
              className="block"
            >
              <DataListRow interactive className="group">
                {/* <div className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-muted/50">
                  <GitBranch className="size-3.5 text-muted-foreground" />
                </div> */}

                <div className="min-w-0">
                  {/* User-supplied text, rendered by React and therefore escaped. */}
                  <div className="truncate text-body font-medium">
                    {project.name}
                  </div>
                  <MonoTag className="text-meta font-sans ">
                    {project.slug}
                  </MonoTag>
                </div>

                <RowMeta>
                  <RelativeTime value={project.createdAt} prefix="Created" />
                  <ChevronRight className="size-3.5 transition-transform group-hover:translate-x-0.5" />
                </RowMeta>
              </DataListRow>
            </Link>
          ))}
        </DataList>
      )}
    </PageShell>
  );
}
