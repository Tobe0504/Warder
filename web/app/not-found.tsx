import Link from "next/link";

import { Mark } from "@/components/logo";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

/**
 * Not found.
 *
 * This page is also what someone sees when they ask for a project that belongs
 * to another organization. The interface never distinguishes "does not exist"
 * from "not yours", because doing so would confirm that an identifier names
 * something real.
 */
export default function NotFound() {
  return (
    <div className="flex min-h-dvh items-center justify-center px-4">
      <Card className="max-w-sm p-5 text-center">
        <Mark size={20} className="mx-auto mb-3" />
        <h1 className="text-body font-medium">Not found</h1>
        <p className="mt-1 text-meta leading-relaxed text-muted-foreground">
          This page does not exist, or it belongs to an organization you are not a member of.
        </p>
        <Button asChild size="sm" className="mt-4">
          <Link href="/projects">Back to projects</Link>
        </Button>
      </Card>
    </div>
  );
}
