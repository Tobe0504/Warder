"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { KeyRound, LayoutGrid, LogOut, Settings } from "lucide-react";

import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/motion/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SignOutDialog } from "@/components/sign-out-dialog";
import { apiFetch } from "@/lib/client-api";

/** Only what the header shows. The endpoint deliberately sends nothing more. */
type HeaderUser = { name: string; email: string; organization: string };

/**
 * The account control in the public header.
 *
 * A client component on purpose. The pages around it are static HTML, so they
 * cannot read a cookie without becoming dynamic, which would cost the landing
 * page and every documentation page their cacheability, and would put a
 * session read in front of visitors who are only here to read the docs.
 *
 * So the page ships signed-out and this asks afterwards. The signed-out state
 * is the real initial state rather than a spinner: most people arriving at a
 * public page are not signed in, and showing them "Sign in" immediately is
 * both correct and the fastest useful paint.
 */
export default function UserAvatar() {
  const [user, setUser] = useState<HeaderUser | null>(null);
  const [confirming, setConfirming] = useState(false);

  useEffect(() => {
    let cancelled = false;

    apiFetch<{ user: HeaderUser | null }>("/api/auth/session")
      .then((result) => {
        if (!cancelled) setUser(result.user);
      })
      .catch(() => {
        // The header stays in its signed-out state, which still works. A
        // failed session lookup is not worth a toast on a marketing page.
      });

    return () => {
      cancelled = true;
    };
  }, []);

  if (!user) {
    return (
      <div className="ml-auto flex items-center gap-1.5">
        <Button asChild variant="ghost" size="sm">
          <Link href="/login">Sign in</Link>
        </Button>
        <Button asChild size="sm">
          <Link href="/signup">Get started</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="ml-auto flex items-center gap-1.5">
      <Button asChild variant="ghost" size="sm" className="max-sm:hidden">
        <Link href="/projects">Dashboard</Link>
      </Button>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            className="rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background"
            aria-label={`Account: ${user.name}`}
          >
            <Avatar name={user.name} />
          </button>
        </DropdownMenuTrigger>

        <DropdownMenuContent side="bottom" align="end" className="w-56">
          <DropdownMenuLabel className="flex items-center gap-2 py-2">
            <Avatar name={user.name} />
            <div className="grid min-w-0 flex-1 leading-tight">
              <span className="truncate text-meta font-medium text-foreground">{user.name}</span>
              <span className="truncate text-meta font-normal">{user.email}</span>
            </div>
          </DropdownMenuLabel>

          <DropdownMenuSeparator />

          <DropdownMenuItem asChild>
            <Link href="/projects">
              <LayoutGrid />
              Dashboard
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href="/identities">
              <KeyRound />
              Identities
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href="/settings">
              <Settings />
              Settings
            </Link>
          </DropdownMenuItem>

          <DropdownMenuSeparator />

          <DropdownMenuItem destructive onSelect={() => setConfirming(true)}>
            <LogOut />
            Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {/*
        A sibling of the menu, not a child: Radix unmounts the menu content
        when the menu closes, and the modal would go with it.
      */}
      <SignOutDialog open={confirming} onOpenChange={setConfirming} />
    </div>
  );
}
