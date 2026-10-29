"use client";

import { useState } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/motion/button";
import { RecoveryCodesPanel } from "@/components/recovery-codes-panel";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";

/**
 * How many recovery codes are left, and issuing a fresh set.
 *
 * Issuing replaces every unused code, so it asks first: someone with a sheet
 * of codes in a drawer would otherwise find it silently worthless.
 */
export function RecoveryCodesSettings({ remaining }: { remaining: number }) {
  const toast = useToast();
  const [count, setCount] = useState(remaining);
  const [codes, setCodes] = useState<string[] | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [pending, setPending] = useState(false);

  async function issue() {
    setPending(true);
    try {
      const result = await apiFetch<{ codes: string[] }>("/api/auth/recovery-codes", {
        method: "POST",
      });
      setCodes(result.codes);
      setCount(result.codes.length);
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "Could not issue new codes.");
    } finally {
      setPending(false);
      setConfirming(false);
    }
  }

  if (codes) {
    return (
      <div className="space-y-3">
        <p className="prose-note text-muted-foreground">
          Save these now. They won&apos;t be shown again, and your old codes no longer work.
        </p>
        <RecoveryCodesPanel codes={codes} />
        <Button type="button" variant="ghost" size="sm" onClick={() => setCodes(null)}>
          I&apos;ve saved them
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <p className="prose-note text-muted-foreground">
        {count === 0
          ? "You have no unused recovery codes. Without them, a lost passkey or password can't be recovered on your own."
          : `${count} unused recovery ${count === 1 ? "code" : "codes"} left. Each one signs you in once if you lose your passkey or password.`}
      </p>
      {confirming ? (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-meta text-muted-foreground">
            This replaces every unused code.
          </span>
          <Button type="button" size="sm" loading={pending} onClick={issue}>
            Issue new codes
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={pending}
            onClick={() => setConfirming(false)}
          >
            Cancel
          </Button>
        </div>
      ) : (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => (count === 0 ? issue() : setConfirming(true))}
          loading={pending}
        >
          <RefreshCw />
          {count === 0 ? "Issue recovery codes" : "Issue new codes"}
        </Button>
      )}
    </div>
  );
}
