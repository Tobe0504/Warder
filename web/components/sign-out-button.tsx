"use client";

import { useState } from "react";
import { LogOut } from "lucide-react";

import { Button } from "@/components/motion/button";
import { SignOutDialog } from "@/components/sign-out-dialog";

export function SignOutButton() {
  const [confirming, setConfirming] = useState(false);

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        onClick={() => setConfirming(true)}
        aria-label="Sign out"
      >
        <LogOut />
      </Button>

      <SignOutDialog open={confirming} onOpenChange={setConfirming} />
    </>
  );
}
