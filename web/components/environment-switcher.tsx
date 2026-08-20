"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { Tabs, TabsList, TabsTrigger } from "@/components/motion/tabs";

type Environment = { id: string; name: string; slug: string };

/**
 * Environment selection.
 *
 * Writes `?env=<slug>` rather than navigating to another route. The page is
 * the same page either way, so a route change made the whole thing unmount and
 * remount to swap one list. `replace` rather than `push` keeps Back meaning
 * "the screen before this one" instead of walking through every environment
 * somebody clicked.
 *
 * Labels use the display name, "Development", not the lowercase mono
 * `development` slug. The slug is an identifier the CLI takes; setting proper
 * nouns in lowercase monospace made a simple choice look like configuration.
 *
 * Nothing here treats "production" as a special word, and the platform does not
 * either. What distinguishes environments is which grants and tokens name them.
 */
export function EnvironmentSwitcher({
  environments,
  currentId,
}: {
  environments: Environment[];
  currentId: string;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();

  const current =
    environments.find((environment) => environment.id === currentId) ??
    environments[0];

  function select(slug: string) {
    const next = new URLSearchParams(params);
    next.set("env", slug);
    router.replace(`${pathname}?${next}`, { scroll: false });
  }

  if (!current) return null;

  return (
    <Tabs value={current.slug} onValueChange={select} variant="segment">
      <TabsList>
        {environments.map((environment) => (
          <TabsTrigger key={environment.id} value={environment.slug}>
            {environment.name}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
}
