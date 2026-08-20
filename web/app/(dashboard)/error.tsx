"use client";

import { useEffect } from "react";
import { RefreshCw, ServerCrash } from "lucide-react";

import { Button } from "@/components/motion/button";
import { Card } from "@/components/ui/card";

/**
 * The dashboard's error boundary.
 *
 * This page exists so that "the core API is not running" is a legible message
 * rather than a stack trace, which is the situation that tempts someone to
 * comment out the session check just to see the interface.
 *
 * Next.js deliberately does not pass the underlying error message to the client
 * in production, and that is right: a server-side error message may quote the
 * internal address it failed to reach. So this renders guidance, not detail.
 * The detail is on the server's console.
 */
export default function DashboardError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // Logged for whoever is at the terminal. In a browser this reaches the
    // developer console only; it is not sent anywhere.
    console.error("Dashboard error:", error);
  }, [error]);

  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Card className="max-w-md p-5">
        <div className="flex items-start gap-3">
          <ServerCrash className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <div className="space-y-3">
            <div>
              <h1 className="text-body font-medium">Could not reach the service</h1>
              <p className="mt-1 text-meta leading-relaxed text-muted-foreground">
                The dashboard is running, but the core API did not answer. Nothing is wrong with
                your account.
              </p>
            </div>

            <div className="space-y-1.5 rounded-md bg-muted px-3 py-2.5">
              <p className="text-meta font-medium">If you are running this locally</p>
              <ol className="space-y-1 prose-note text-muted-foreground">
                <li>
                  1. Is the database up?{" "}
                  <code className="font-mono">docker compose -f deploy/docker-compose.yml ps</code>
                </li>
                <li>
                  2. Is the API running?{" "}
                  <code className="font-mono">go run ./cmd/warder-api serve</code>
                </li>
                <li>
                  3. Does <code className="font-mono">web/.env.local</code> have a{" "}
                  <code className="font-mono">WARDER_URL</code> pointing at it?
                </li>
              </ol>
            </div>

            <div className="flex items-center gap-2">
              <Button size="sm" onClick={reset}>
                <RefreshCw />
                Try again
              </Button>
              {error.digest && (
                // The digest correlates this page with the server log line that
                // has the real cause. It is opaque and safe to display.
                <span className="font-mono text-meta text-muted-foreground">
                  ref {error.digest}
                </span>
              )}
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
}
