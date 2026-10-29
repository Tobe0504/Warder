import Link from "next/link";

import { AuthShell } from "@/components/auth-shell";
import { ForgotPasswordForm } from "@/components/forgot-password-form";

export const metadata = { title: "Forgot password" };

export default function ForgotPasswordPage() {
  return (
    <AuthShell
      title="Use a recovery code"
      description="Warder sends no email, so there is no reset link. A recovery code from the set issued when you signed up takes its place."
      footer={
        <>
          Lost those too?{" "}
          <span className="text-foreground">
            An owner of your organization can issue you a recovery link.
          </span>{" "}
          <Link
            href="/login"
            className="text-foreground underline underline-offset-4 hover:opacity-70"
          >
            Back to sign in
          </Link>
        </>
      }
    >
      <ForgotPasswordForm />
    </AuthShell>
  );
}
