import Link from "next/link";

import { Crumb, CrumbSeparator, PageShell } from "@/components/page-shell";

/**
 * The frame around every page inside a project.
 *
 * The header belongs to the page rather than to the layout, because the action
 * in it does: "Add secrets" is the secrets page's, "Grant access" is the access
 * page's, and a layout cannot be handed either by a server-rendered child.
 * Pages that share a breadcrumb share it through here instead.
 */
export function ProjectShell({
  project,
  section,
  actions,
  children,
}: {
  project: { id: string; name: string; slug: string };
  /** The section's own name, shown after the project. Omit on the index. */
  section?: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <PageShell
      actions={actions}
      breadcrumb={
        <>
          <Link
            href="/projects"
            className="rounded text-muted-foreground hover:text-foreground"
          >
            Projects
          </Link>
          <CrumbSeparator />
          {section ? (
            <>
              <Link
                href={`/projects/${project.id}`}
                className="rounded truncate text-muted-foreground hover:text-foreground"
              >
                {project.name}
              </Link>
              <CrumbSeparator />
              <Crumb>{section}</Crumb>
            </>
          ) : (
            <Crumb>{project.name}</Crumb>
          )}
        </>
      }
    >
      {children}
    </PageShell>
  );
}
