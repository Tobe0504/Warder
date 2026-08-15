import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight, Eye, EyeOff, Terminal } from "lucide-react";

import { Button } from "@/components/ui/button";

export const metadata: Metadata = {
  // `absolute` opts out of the root layout's "%s, Warder" template, which
  // would otherwise name the product twice on the one page that leads with it.
  title: { absolute: "Warder, use credentials without seeing them" },
  description:
    "A secret access broker. Applications receive the credentials they need; the people and agents who build those applications do not.",
};

export default function LandingPage() {
  return (
    <main>
      <Hero />
      <TwoPermissions />
      <WhatItBuys />
      <Integration />
      <Honesty />
      <ClosingCta />
    </main>
  );
}

/* -------------------------------------------------------------------------- */

function Hero() {
  return (
    <section className="mx-auto max-w-7xl px-4 pb-14 pt-16 sm:px-6 sm:pt-14">
      <h1 className="mt-4 max-w-[18ch] text-[2.25rem] font-medium leading-[1.08] tracking-[-0.03em] sm:text-[3.25rem]">
        Your application gets the credential. Nobody else has to.
      </h1>

      <p className="mt-5 max-w-[58ch] text-body leading-relaxed text-muted-foreground sm:text-[1.0625rem]">
        Warder separates <em className="not-italic text-foreground">using</em> a
        secret from <em className="not-italic text-foreground">seeing</em> one.
        Developers run the app, CI deploys it, agents test it: none of them are
        handed the value, and none of them need to be.
      </p>

      <div className="mt-8 flex flex-wrap items-center gap-2">
        <Button asChild size="lg">
          <Link href="/docs/using-warder">
            Read the docs
            <ArrowRight />
          </Link>
        </Button>
        <Button asChild size="lg" variant="outline">
          <Link href="/signup">Get started</Link>
        </Button>
      </div>

      <SplitDemo />
    </section>
  );
}

/**
 * The signature element: one command, two very different views of the result.
 *
 * This is the whole product in one picture, and it is the picture the README
 * has always drawn in ASCII. The left pane is what the person who typed the
 * command can see. The right is what the process they started actually holds.
 * The gap between those two panes is the product.
 */
function SplitDemo() {
  return (
    <div className="mt-14 overflow-hidden rounded-xl border bg-card">
      <div className="flex items-center gap-2 border-b px-4 py-2.5">
        <Terminal className="size-3.5 text-muted-foreground" />
        <code className="font-mono text-meta">
          <span className="text-muted-foreground">$ </span>
          ward run -- npm run dev
        </code>
      </div>

      <div className="grid divide-y sm:grid-cols-2 sm:divide-x sm:divide-y-0">
        <Pane
          icon={<EyeOff className="size-3.5" />}
          label="What you can see"
          tone="reveal"
          rows={[
            ["DATABASE_URL", "••••••••••••••••••••••"],
            ["STRIPE_SECRET_KEY", "••••••••••••••••"],
            ["AUTH_SECRET", "••••••••••••••••••"],
          ]}
          note="Names, versions, expiry. Never the value, unless someone explicitly granted you that, and it is recorded when you use it."
        />

        <Pane
          icon={<Eye className="size-3.5" />}
          label="What the process receives"
          tone="use"
          rows={[
            ["DATABASE_URL", "postgres://…@db.internal:5432/app"],
            ["STRIPE_SECRET_KEY", "sk_live_51H8xK2eZvKYlo2C…"],
            ["AUTH_SECRET", "8f3c9a1d4e7b2f6a0c5d8e3b…"],
          ]}
          note="Delivered into the environment block of the process you started. Not to your terminal, not to a file, not to your shell history."
        />
      </div>
    </div>
  );
}

function Pane({
  icon,
  label,
  tone,
  rows,
  note,
}: {
  icon: React.ReactNode;
  label: string;
  tone: "use" | "reveal";
  rows: [string, string][];
  note: string;
}) {
  const accent = tone === "use" ? "text-can-use" : "text-muted-foreground";

  return (
    <div className="p-4">
      <div
        className={`flex items-center gap-1.5 text-micro font-medium uppercase ${accent}`}
      >
        {icon}
        {label}
      </div>

      <dl className="mt-3.5 space-y-2">
        {rows.map(([key, value]) => (
          <div key={key} className="min-w-0">
            <dt className="truncate font-mono text-meta text-muted-foreground">
              {key}
            </dt>
            <dd
              className={`truncate font-mono text-meta ${
                tone === "use" ? "text-can-use" : "text-muted-foreground/70"
              }`}
            >
              {value}
            </dd>
          </div>
        ))}
      </dl>

      <p className="mt-4 border-t pt-3 text-meta leading-relaxed text-muted-foreground">
        {note}
      </p>
    </div>
  );
}

/* -------------------------------------------------------------------------- */

function TwoPermissions() {
  return (
    <Section
      eyebrow="The model"
      title="Two permissions, and no role grants either one"
      lede="Most tools have a single “access” permission, so anyone who can deploy can also read. Warder splits it, and neither half comes with a job title."
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <article className="rounded-xl border bg-card p-5">
          <h3 className="text-micro font-medium uppercase text-can-use">
            Can use
          </h3>
          <p className="mt-2.5 text-body font-medium">
            The value reaches a process, and no human reads it.
          </p>
          <p className="mt-2 prose-note text-muted-foreground">
            What you grant your API server, your CI job, your test suite, an
            agent. It is the ordinary case, and it is the one that lets a
            credential do its job without becoming common knowledge.
          </p>
        </article>

        <article className="rounded-xl border bg-card p-5">
          <h3 className="text-micro font-medium uppercase text-can-reveal">
            Can see
          </h3>
          <p className="mt-2.5 text-body font-medium">
            A named person displays the plaintext, and it is recorded.
          </p>
          <p className="mt-2 prose-note text-muted-foreground">
            Sometimes genuinely necessary: pasting a key into a third-party
            console that has no other way in. It is a separate, deliberate
            grant, and every use of it has a name and a timestamp attached.
          </p>
        </article>
      </div>

      <p className="mt-4 prose-note text-muted-foreground">
        Owner, Admin, Developer, Viewer, those govern administration: creating
        projects, managing members, reading the audit log.{" "}
        <strong className="font-medium text-foreground">
          None of them confers the ability to use or see a single secret value.
        </strong>{" "}
        That is always an explicit grant on a specific environment, which is
        what makes the answer to “who can read our production database
        password?” a short list instead of a guess.
      </p>
    </Section>
  );
}

/* -------------------------------------------------------------------------- */

const SCENARIOS: [string, React.ReactNode][] = [
  [
    "A contractor finishes and leaves.",
    <>
      Revoke their membership.{" "}
      <strong className="font-medium text-foreground">Nothing rotates</strong>:
      they could use the database, they never held it.
    </>,
  ],
  [
    "An agent gets prompt-injected by a dependency README.",
    <>
      It holds{" "}
      <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.9em]">
        USE_SECRET
      </code>{" "}
      on development. It cannot print a value, reach production, rotate
      anything, or grant itself more.
    </>,
  ],
  [
    "Someone pushed a .env to a public repository.",
    <>
      There wasn’t one. The file holds two publishable keys and a project name.
    </>,
  ],
  [
    "You need to rotate a database password.",
    <>
      Rotate it in the dashboard and restart. No CI config, no manifests, no
      messages to the team.
    </>,
  ],
  [
    "Security asks who has read this credential.",
    <>One query against an append-only log. Usually the answer is nobody.</>,
  ],
  [
    "You revoke a token during an incident.",
    <>
      The next request is denied, including short-lived sessions already
      derived from it, rather than waiting for them to expire.
    </>,
  ],
];

function WhatItBuys() {
  return (
    <Section
      eyebrow="In practice"
      title="What that actually changes"
      lede="The same situations, with a different amount of work in them."
    >
      <div className="grid gap-x-8 gap-y-6 sm:grid-cols-2">
        {SCENARIOS.map(([question, answer]) => (
          <div key={question} className="border-t pt-4">
            <p className="text-body font-medium">{question}</p>
            <p className="mt-1.5 prose-note text-muted-foreground">{answer}</p>
          </div>
        ))}
      </div>
    </Section>
  );
}

/* -------------------------------------------------------------------------- */

const STEPS: [string, string, string][] = [
  [
    "ward init --project payments-api --env development",
    "Point the repository at an environment",
    "Writes .warder.json. Two names, no credentials: commit it.",
  ],
  [
    "ward login",
    "Sign in once per machine",
    "Stores a session in your home directory. Developers set no environment variables at all.",
  ],
  [
    "ward run -- npm run dev",
    "Run anything with the secrets injected",
    "Your code keeps reading process.env. No SDK, no imports, no code changes.",
  ],
];

function Integration() {
  return (
    <Section
      eyebrow="Integration"
      title="Three commands, and your code stays the same"
      lede="Warder delivers values into the environment your process already reads. There is nothing to import and nothing to rewrite."
    >
      <ol className="space-y-3">
        {STEPS.map(([command, label, hint], index) => (
          <li
            key={command}
            className="flex gap-3 rounded-xl border bg-card p-4"
          >
            <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full border text-micro tabular">
              {index + 1}
            </span>
            <div className="min-w-0">
              <p className="text-body font-medium">{label}</p>
              <code className="mt-1.5 block break-all font-mono text-meta text-muted-foreground">
                {command}
              </code>
              <p className="mt-1.5 text-meta leading-relaxed text-muted-foreground">
                {hint}
              </p>
            </div>
          </li>
        ))}
      </ol>

      <p className="mt-4 prose-note text-muted-foreground">
        In production the same command runs, with the credential coming from
        your platform&rsquo;s secret store instead of a login.{" "}
        <Link
          href="/docs/using-warder"
          className="font-medium text-foreground underline underline-offset-2 decoration-border hover:decoration-foreground"
        >
          Full walkthrough, including frontends and CI
        </Link>
        .
      </p>
    </Section>
  );
}

/* -------------------------------------------------------------------------- */

/**
 * The section that says what this does not do.
 *
 * On a security product this is not modesty, it is load-bearing. Someone
 * deciding whether to trust this needs the boundary stated before they adopt
 * it, not discovered afterwards, and a page that only claims wins is a page
 * that cannot be checked.
 */
function Honesty() {
  return (
    <Section
      eyebrow="Limits"
      title="What Warder does not do"
      lede="Stated here rather than buried, because a security tool that only advertises its wins is hard to evaluate."
    >
      <ul className="space-y-4">
        {[
          [
            "It does not get you to zero credentials.",
            "Your application still holds one token that proves which application it is. Warder makes that one credential scoped, expiring and instantly revocable; it does not make it disappear. Nobody has solved that.",
          ],
          [
            "It does not stop an authorized process from reading its own environment.",
            "A process given DATABASE_URL can print it. The protection is that the grant was narrow and deliberate, not that the value is unreadable once delivered.",
          ],
          [
            "It does not protect values you ship to a browser.",
            "Anything in a client bundle is public no matter what produced it. Warder manages the server half and says so.",
          ],
        ].map(([claim, detail]) => (
          <li key={claim} className="border-t pt-4">
            <p className="text-body font-medium">{claim}</p>
            <p className="mt-1.5 prose-note text-muted-foreground">{detail}</p>
          </li>
        ))}
      </ul>

      <div className="mt-6">
        <Button asChild variant="outline" size="sm">
          <Link href="/docs/security/limitations">
            Read the full limitations
            <ArrowRight />
          </Link>
        </Button>
      </div>
    </Section>
  );
}

/* -------------------------------------------------------------------------- */

function ClosingCta() {
  return (
    <section className="border-t">
      <div className="mx-auto max-w-7xl px-4 py-16 sm:px-6">
        <h2 className="max-w-[30ch] text-title font-medium tracking-tight sm:text-[1.75rem]">
          A credential should be available to the software that needs it, and
          to nobody else by default.
        </h2>
        <div className="mt-6 flex flex-wrap gap-2">
          <Button asChild>
            <Link href="/docs">
              Browse the documentation
              <ArrowRight />
            </Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/docs/architecture">How it works</Link>
          </Button>
        </div>
      </div>
    </section>
  );
}

/* -------------------------------------------------------------------------- */

function Section({
  eyebrow,
  title,
  lede,
  children,
}: {
  eyebrow: string;
  title: string;
  lede: string;
  children: React.ReactNode;
}) {
  return (
    <section className="border-t">
      <div className="mx-auto max-w-7xl px-4 py-16 sm:px-6">
        <p className="text-micro font-medium uppercase text-muted-foreground">
          {eyebrow}
        </p>
        <h2 className="mt-3 max-w-[24ch] text-title font-medium tracking-tight sm:text-[1.75rem]">
          {title}
        </h2>
        <p className="mt-3 mb-8 max-w-[62ch] prose-note text-muted-foreground">
          {lede}
        </p>
        {children}
      </div>
    </section>
  );
}
