/**
 * Who is signed in.
 *
 * Deliberately its own module rather than an export of the dashboard layout.
 * A component importing this type from `app/(dashboard)/layout` pulls that
 * route's whole module graph, the sidebar, the core API client, `force-dynamic`
 *: into whatever imported it, and when the importer is itself rendered by
 * another layout the result is a circular module graph that fails at runtime
 * with an unhelpful "Cannot read properties of undefined (reading 'call')".
 *
 * Types are the one thing several layers legitimately share, so they live
 * somewhere neutral that imports nothing.
 */
export type SessionUser = {
  id: string;
  email: string;
  name: string;
  organizationId: string;
  organization: string;
  role: string;
};
