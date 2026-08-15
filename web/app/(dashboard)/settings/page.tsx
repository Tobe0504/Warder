import { KeyRound, Server, ShieldCheck } from "lucide-react";

import { Crumb, PageShell, PageTitle } from "@/components/page-shell";
import { Badge } from "@/components/ui/badge";
import { DataList, DataListRow } from "@/components/ui/data-list";
import { callCoreApi } from "@/lib/core-api";

type SessionUser = {
  email: string;
  name: string;
  organization: string;
  organizationId: string;
  role: string;
};

/**
 * Settings.
 *
 * Read-only for now, and honest about that. The security posture summary is
 * the useful part: it states which guarantees this deployment is actually
 * providing, so an operator does not have to infer them from documentation.
 */
export default async function SettingsPage() {
  const user = await callCoreApi<SessionUser>("/auth/session");

  return (
    <PageShell breadcrumb={<Crumb>Settings</Crumb>}>
      <PageTitle title="Settings" description="Your account and this organization." />

      <div className="space-y-8">
        <section>
          <h2 className="mb-3 text-heading font-medium">Account</h2>
          <DataList>
            <Field label="Name" value={user.name} />
            <Field label="Email" value={user.email} />
            <Field label="Organization" value={user.organization} />
            <Field
              label="Role"
              value={
                <span className="inline-flex items-center gap-2">
                  <Badge variant="outline">{user.role.toLowerCase()}</Badge>
                  <span className="text-meta text-muted-foreground">
                    administration only: grants no access to any secret value
                  </span>
                </span>
              }
            />
          </DataList>
        </section>

        <section>
          <h2 className="mb-3 text-heading font-medium">Security posture</h2>
          <DataList>
            <Posture
              icon={<KeyRound className="size-3.5" />}
              title="Envelope encryption"
              detail="Every value is sealed with a per-version data key, wrapped by a key that is never stored in the database."
            />
            <Posture
              icon={<ShieldCheck className="size-3.5" />}
              title="No role grants plaintext"
              detail="READ_SECRET always comes from an explicit, audited grant. Owners included."
            />
            <Posture
              icon={<Server className="size-3.5" />}
              title="Separate runtime surface"
              detail="Secret delivery runs on its own listener, which this dashboard cannot reach."
            />
          </DataList>
        </section>

        <p className="prose-note text-muted-foreground">
          Editing organization details, rotating the encryption key, and managing members from this
          screen are not built yet. See <span className="font-mono">docs/security/limitations.md</span>{" "}
          for the full list of what this build does not do.
        </p>
      </div>
    </PageShell>
  );
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <DataListRow>
      <span className="w-32 shrink-0 text-meta text-muted-foreground">{label}</span>
      <span className="min-w-0 flex-1 truncate text-body">{value}</span>
    </DataListRow>
  );
}

function Posture({
  icon,
  title,
  detail,
}: {
  icon: React.ReactNode;
  title: string;
  detail: string;
}) {
  return (
    <DataListRow>
      <div className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-muted/50 text-muted-foreground">
        {icon}
      </div>
      <div className="min-w-0">
        <div className="text-body">{title}</div>
        <div className="mt-0.5 prose-note text-muted-foreground">{detail}</div>
      </div>
    </DataListRow>
  );
}
