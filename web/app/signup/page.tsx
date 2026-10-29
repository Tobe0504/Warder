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
      title="Create your account"
      description="An account on its own belongs to nothing. The next step is to create an organization or open an invitation to one."
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
