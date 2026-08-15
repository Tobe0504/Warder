import Link from "next/link";
import { redirect } from "next/navigation";

import { AuthShell } from "@/components/auth-shell";
import { SignupForm } from "@/components/signup-form";
import { getSessionToken } from "@/lib/session";

export default async function SignupPage() {
  if (await getSessionToken()) {
    redirect("/projects");
  }

  return (
    <AuthShell
      title="Create an organization"
      description="You will be its first owner, which lets you administer projects and manage access, and does not by itself let you read any secret value."
      footer={
        <>
          Already have an account?{" "}
          <Link
            href="/login"
            className="text-foreground underline underline-offset-4 hover:opacity-70"
          >
            Sign in
          </Link>
        </>
      }
    >
      <SignupForm />
    </AuthShell>
  );
}
