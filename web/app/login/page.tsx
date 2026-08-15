import Link from "next/link";
import { redirect } from "next/navigation";
import { AlertTriangle } from "lucide-react";

import { AuthShell } from "@/components/auth-shell";
import { LoginForm } from "@/components/login-form";
import { configurationProblem } from "@/lib/env";
import { getSessionToken } from "@/lib/session";

export default async function LoginPage() {
  // Checked before anything else, so the very first page a new developer loads
  // tells them their configuration is wrong instead of failing on submit.
  const problem = configurationProblem();

  if (!problem && (await getSessionToken())) {
    redirect("/projects");
  }

  if (problem) {
    return (
      <AuthShell
        title="Not configured yet"
        description="The dashboard needs a connection to the core API before anyone can sign in."
      >
        <div className="flex gap-2.5">
          <AlertTriangle className="mt-0.5 size-4 shrink-0 text-can-reveal" />
          {/*
            The parser's message names what is wrong and how to fix it, and is
            written never to quote the credential itself.
          */}
          <pre className="min-w-0 flex-1 overflow-x-auto whitespace-pre-wrap font-mono prose-note text-muted-foreground">
            {problem}
          </pre>
        </div>
      </AuthShell>
    );
  }

  return (
    <AuthShell
      title="Sign in to Warder"
      description="Use credentials without seeing them."
      footer={
        <>
          First time here?{" "}
          <Link
            href="/signup"
            className="text-foreground underline underline-offset-4 hover:opacity-70"
          >
            Create an organization
          </Link>
        </>
      }
    >
      <LoginForm />
    </AuthShell>
  );
}
