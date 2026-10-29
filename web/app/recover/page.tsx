import Link from "next/link";

import { AuthShell } from "@/components/auth-shell";
import { RedeemRecoveryForm } from "@/components/redeem-recovery-form";

export const metadata = { title: "Recover your account" };

/**
 * Redeeming a link an owner issued.
 *
 * The token arrives in the URL fragment, which is never sent to a server: the
 * same reasoning as invitations. It keeps the link out of this application's
 * access log, out of any proxy in front of it, and out of the Referer header
 * of whatever the person clicks next.
 */
export default function RecoverPage() {
  return (
    <AuthShell
      title="Set a new password"
      description="This link was issued by an owner of your organization. It works once, and every session on your account signs out when you use it."
      footer={
        <Link
          href="/login"
          className="text-foreground underline underline-offset-4 hover:opacity-70"
        >
          Back to sign in
        </Link>
      }
    >
      <RedeemRecoveryForm />
    </AuthShell>
  );
}
