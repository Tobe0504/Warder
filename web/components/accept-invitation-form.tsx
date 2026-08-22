"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Input } from "@/components/motion/input";
import { apiFetch } from "@/lib/client-api";

/**
 * Redeems an invitation.
 *
 * The token arrives in the URL fragment, which is why this has to be a client
 * component: a fragment is never sent to a server. That is the point of putting
 * it there: the invitation stays out of this application's access log, out of
 * any proxy in front of it, and out of the Referer header of whatever the
 * invitee clicks next.
 *
 * It is read once on mount and then removed from the address bar, so it does
 * not sit in browser history or get shoulder-read from a URL bar.
 */
type Invitation = {
  organization: string;
  role: string;
  email: string;
  /** The address already has a Warder account. */
  accountExists: boolean;
  /** This browser is signed in as that address, so it can accept right now. */
  canClaim: boolean;
};

export function AcceptInvitationForm() {
  const router = useRouter();

  const [token, setToken] = useState<string | null>(null);
  const [ready, setReady] = useState(false);

  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [done, setDone] = useState<string | null>(null);
  const [joined, setJoined] = useState(false);
  const [invitation, setInvitation] = useState<Invitation | null>(null);

  useEffect(() => {
    const fragment = window.location.hash.replace(/^#/, "").trim();
    if (!fragment) {
      setReady(true);
      return;
    }
    setToken(fragment);

    let cancelled = false;

    // Asked before anything is rendered, so the page can offer the one action
    // that applies. Without it the only way to discover that this address
    // already has an account is to fill in a password and have it refused.
    apiFetch<Invitation>("/api/auth/invitations/inspect", {
      method: "POST",
      body: { token: fragment },
    })
      .then((result) => {
        if (cancelled) return;
        setInvitation(result);

        // The fragment stays in the address bar in exactly one case: the
        // address has an account and this browser is not signed in as it. That
        // person has to leave for the sign-in page and come back, and the
        // alternative to keeping it here is putting a live invitation token in
        // browser storage, which is the one place it must never go.
        if (!(result.accountExists && !result.canClaim)) {
          window.history.replaceState(null, "", window.location.pathname);
        }
      })
      .catch((caught) => {
        if (cancelled) return;
        window.history.replaceState(null, "", window.location.pathname);
        setError(
          caught instanceof Error
            ? caught.message
            : "This invitation could not be read.",
        );
      })
      .finally(() => {
        if (!cancelled) setReady(true);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    if (password !== confirm) {
      setError("Those passwords do not match.");
      return;
    }
    if (!token) {
      setError("This link is missing its invitation. Ask for a new one.");
      return;
    }

    setPending(true);
    try {
      const result = await apiFetch<{ email: string }>(
        "/api/auth/accept-invitation",
        {
          method: "POST",
          body: { token, name, password },
        },
      );
      setDone(result.email);
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "This invitation could not be used.",
      );
    } finally {
      setPending(false);
    }
  }

  /**
   * Claims the invitation with the account already signed in on this browser.
   *
   * The request carries no name and no password: the account exists, and the
   * server reads the address off the session rather than the form, so this
   * cannot be used to join as somebody else.
   */
  async function join() {
    if (!token) return;
    setPending(true);
    setError(null);
    try {
      await apiFetch("/api/auth/accept-invitation", {
        method: "POST",
        body: { token },
      });
      setJoined(true);
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "This invitation could not be used.",
      );
    } finally {
      setPending(false);
    }
  }

  if (!ready) {
    return (
      <p className="text-center text-meta text-muted-foreground">
        Checking the link…
      </p>
    );
  }

  if (joined) {
    return (
      <div className="text-center">
        <p className="prose-note text-muted-foreground">
          You have joined. Switch between organizations from the name at the top
          of the sidebar.
        </p>
        <Button
          className="mt-4 w-full"
          onClick={() => {
            // A full load, not a router push: the session now carries a
            // different organization and every cached payload predates it.
            window.location.href = "/projects";
          }}
        >
          Go to the dashboard
          <ArrowRight />
        </Button>
      </div>
    );
  }

  // Signed in as the invited address: one button, nothing to type.
  if (invitation?.canClaim) {
    return (
      <div className="space-y-4 text-center">
        <p className="prose-note text-muted-foreground">
          Join <span className="font-medium text-foreground">
            {invitation.organization}
          </span>{" "}
          as{" "}
          <span className="font-medium text-foreground">
            {invitation.email}
          </span>
          , with the role of {invitation.role.toLowerCase()}.
        </p>

        {error && (
          <p role="alert" className="text-meta text-destructive">
            {error}
          </p>
        )}

        <Button className="w-full" onClick={join} loading={pending}>
          Join {invitation.organization}
        </Button>

        <p className="text-meta text-muted-foreground">
          This adds an organization to the account you already have. Your
          password does not change; there is only ever one.
        </p>
      </div>
    );
  }

  // Has an account, but not signed in here.
  if (invitation?.accountExists) {
    return (
      <div className="space-y-4 text-center">
        <p className="prose-note text-muted-foreground">
          <span className="font-medium text-foreground">
            {invitation.email}
          </span>{" "}
          already has a Warder account, so there is no second one to create.
          Sign in, then come back to this page to join{" "}
          <span className="font-medium text-foreground">
            {invitation.organization}
          </span>
          .
        </p>

        <Button asChild className="w-full">
          <Link href="/login">
            Sign in
            <ArrowRight />
          </Link>
        </Button>

        <p className="text-meta text-muted-foreground">
          Your browser&rsquo;s back button returns you here with the invitation
          intact.
        </p>
      </div>
    );
  }

  if (done) {
    return (
      <div className="text-center">
        <p className="prose-note text-muted-foreground">
          Your account is ready. Sign in as{" "}
          <span className="font-medium text-foreground">{done}</span> with the
          password you just chose.
        </p>
        <Button className="mt-4 w-full" onClick={() => router.push("/login")}>
          Go to sign in
          <ArrowRight />
        </Button>
      </div>
    );
  }

  if (!token) {
    return (
      <div className="text-center">
        <p role="alert" className="prose-note text-muted-foreground">
          This link does not carry an invitation. It may have been shortened,
          forwarded as plain text, or truncated by a chat client, the part after
          the <span className="font-mono">#</span> matters. Ask whoever invited
          you for a fresh link.
        </p>
        <Button asChild variant="outline" className="mt-4 w-full">
          <Link href="/login">Sign in instead</Link>
        </Button>
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="space-y-4">
      {invitation && (
        <p className="prose-note text-center text-muted-foreground">
          Creating an account for{" "}
          <span className="font-medium text-foreground">
            {invitation.email}
          </span>{" "}
          to join{" "}
          <span className="font-medium text-foreground">
            {invitation.organization}
          </span>
          .
        </p>
      )}

      <Input
        id="accept-name"
        label="Your name"
        value={name}
        onChange={setName}
        placeholder="Ada Lovelace"
        autoComplete="name"
        required
      />

      <div className="space-y-1.5">
        <Input
          id="accept-password"
          label="Choose a password"
          type="password"
          value={password}
          onChange={setPassword}
          autoComplete="new-password"
          minLength={12}
          required
        />
        <p className="px-1 prose-note text-muted-foreground">
          At least 12 characters. Nobody else sees this, not even your
          organization owner.
        </p>
      </div>

      <Input
        id="accept-confirm"
        label="Confirm password"
        type="password"
        value={confirm}
        onChange={setConfirm}
        autoComplete="new-password"
        minLength={12}
        required
      />

      {error && (
        <p role="alert" className="text-meta text-destructive">
          {error}
        </p>
      )}

      <Button type="submit" className="w-full" loading={pending}>
        Accept invitation
      </Button>
    </form>
  );
}
