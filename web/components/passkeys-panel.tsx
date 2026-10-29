"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Fingerprint, Plus, Trash2 } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";
import {
  DataList,
  DataListRow,
  EmptyState,
  RowMeta,
} from "@/components/ui/data-list";
import { RelativeTime } from "@/components/ui/relative-time";
import { useToast } from "@/components/ui/toast";
import { apiFetch } from "@/lib/client-api";
import {
  createCredential,
  describePasskeyError,
  passkeysSupported,
} from "@/lib/webauthn";

export type Passkey = {
  id: string;
  nickname: string;
  createdAt: string;
  lastUsedAt: string | null;
  transports: string[];
};

/**
 * The caller's passkeys.
 *
 * Adding one turns it into a second factor at the next sign-in, so the copy
 * says so before the prompt appears rather than after.
 */
export function PasskeysPanel({
  passkeys,
  enabled,
}: {
  passkeys: Passkey[];
  enabled: boolean;
}) {
  const router = useRouter();
  const toast = useToast();
  const [adding, setAdding] = useState(false);
  const [nickname, setNickname] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [supported, setSupported] = useState(true);

  useEffect(() => setSupported(passkeysSupported()), []);

  async function register(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setPending(true);

    try {
      const options = await apiFetch<Parameters<typeof createCredential>[0]>(
        "/api/auth/passkeys/register/begin",
        { method: "POST" },
      );
      const credential = await createCredential(options);
      await apiFetch("/api/auth/passkeys/register/finish", {
        method: "POST",
        body: { nickname: nickname.trim(), credential },
      });

      toast.success(`${nickname.trim()} added. You'll be asked for it when you sign in.`);
      setNickname("");
      setAdding(false);
      router.refresh();
    } catch (caught) {
      setError(describePasskeyError(caught));
    } finally {
      setPending(false);
    }
  }

  if (!enabled) {
    return (
      <DataList>
        <EmptyState
          icon={<Fingerprint className="size-5" />}
          title="Passkeys are off on this deployment"
          description="An operator needs to set WARDER_WEBAUTHN_RP_ID and WARDER_WEBAUTHN_ORIGINS before passkeys can be used."
        />
      </DataList>
    );
  }

  return (
    <div className="space-y-3">
      <DataList>
        {passkeys.length === 0 ? (
          <EmptyState
            icon={<Fingerprint className="size-5" />}
            title="No passkeys"
            description="Add one to require it alongside your password, or to sign in without a password at all."
          />
        ) : (
          passkeys.map((passkey) => (
            <PasskeyRow
              key={passkey.id}
              passkey={passkey}
              isLast={passkeys.length === 1}
            />
          ))
        )}
      </DataList>

      {!supported ? (
        <p className="prose-note text-muted-foreground">
          This browser can&apos;t create passkeys.
        </p>
      ) : adding ? (
        <form onSubmit={register} className="space-y-3 rounded-lg border bg-card p-4">
          <Input
            id="passkey-nickname"
            name="nickname"
            label="Name"
            placeholder="MacBook Touch ID"
            autoFocus
            required
            maxLength={100}
            value={nickname}
            onChange={setNickname}
          />
          {error && (
            <p role="alert" className="text-meta text-destructive">
              {error}
            </p>
          )}
          <div className="flex gap-2">
            <Button type="submit" size="sm" loading={pending}>
              Continue
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={pending}
              onClick={() => {
                setAdding(false);
                setError(null);
              }}
            >
              Cancel
            </Button>
          </div>
        </form>
      ) : (
        <Button type="button" variant="outline" size="sm" onClick={() => setAdding(true)}>
          <Plus />
          Add passkey
        </Button>
      )}
    </div>
  );
}

function PasskeyRow({ passkey, isLast }: { passkey: Passkey; isLast: boolean }) {
  const router = useRouter();
  const toast = useToast();
  const [confirming, setConfirming] = useState(false);
  const [pending, setPending] = useState(false);

  async function remove() {
    setPending(true);
    try {
      const result = await apiFetch<{ remaining: number }>(
        `/api/auth/passkeys/${passkey.id}`,
        { method: "DELETE" },
      );
      toast.success(
        result.remaining === 0
          ? `${passkey.nickname} removed. Your password alone now signs you in.`
          : `${passkey.nickname} removed.`,
      );
      router.refresh();
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "Could not remove this passkey.");
    } finally {
      setPending(false);
      setConfirming(false);
    }
  }

  return (
    <DataListRow>
      <div className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-muted/50 text-muted-foreground">
        <Fingerprint className="size-3.5" />
      </div>
      <div className="min-w-0">
        <div className="truncate text-body">{passkey.nickname}</div>
        <div className="mt-0.5 text-meta text-muted-foreground">
          Added <RelativeTime value={passkey.createdAt} />
        </div>
      </div>
      <RowMeta>
        <RelativeTime value={passkey.lastUsedAt} prefix="Used" fallback="Never used" />
        {confirming ? (
          <span className="flex items-center gap-1.5">
            <span className="hidden text-meta text-muted-foreground sm:inline">
              {isLast ? "Last passkey. Remove?" : "Remove?"}
            </span>
            <Button
              type="button"
              variant="destructive"
              size="sm"
              loading={pending}
              onClick={remove}
            >
              Remove
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={pending}
              onClick={() => setConfirming(false)}
            >
              Keep
            </Button>
          </span>
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`Remove ${passkey.nickname}`}
            onClick={() => setConfirming(true)}
          >
            <Trash2 />
          </Button>
        )}
      </RowMeta>
    </DataListRow>
  );
}
