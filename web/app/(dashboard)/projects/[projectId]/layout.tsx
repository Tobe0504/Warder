import { notFound } from "next/navigation";

import { callCoreApi, CoreApiError } from "@/lib/core-api";

type Params = { params: Promise<{ projectId: string }> };

type Project = {
  id: string;
  name: string;
  slug: string;
};

/**
 * Guards the project, and nothing else.
 *
 * The sections live in the sidebar now, and each page draws its own header so
 * that the action belonging to that page lands in it. What is left here is the
 * one thing a layout is still the right place for: refusing a project this
 * session may not see, once, for every page under it.
 */
export default async function ProjectLayout({
  children,
  params,
}: { children: React.ReactNode } & Params) {
  const { projectId } = await params;

  try {
    await callCoreApi<Project>(`/projects/${projectId}`);
  } catch (error) {
    // A project in another organization answers 404 from the core API, and it
    // renders as "not found" here too. The interface never distinguishes
    // "someone else's" from "does not exist".
    if (error instanceof CoreApiError && error.status === 404) {
      notFound();
    }
    throw error;
  }

  return <>{children}</>;
}
