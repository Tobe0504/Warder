"use client";

import { useState } from "react";
import { Check, Copy, Download } from "lucide-react";

import { Button } from "@/components/motion/button";
import { cn } from "@/lib/utils";

/**
 * Ten codes, shown once.
 *
 * They are stored hashed, so there is no screen anywhere that can show them
 * again. That is the property the copy has to carry: this is not a
 * notification, it is the only chance.
 */
export function RecoveryCodesPanel({
  codes,
  className,
}: {
  codes: string[];
  className?: string;
}) {
  const [copied, setCopied] = useState(false);

  const asText = codes.join("\n");

  async function copy() {
    await navigator.clipboard.writeText(asText);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  function download() {
    // A blob rather than a link to the server: these never touch a URL, an
    // access log or a proxy.
    const blob = new Blob(
      [
        "Warder recovery codes\n",
        "Each one works once. Keep them somewhere you can reach without this account.\n\n",
        `${asText}\n`,
      ],
      { type: "text/plain" },
    );
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "warder-recovery-codes.txt";
    link.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className={cn("rounded-lg border bg-card p-4", className)}>
      <ul className="grid grid-cols-2 gap-x-6 gap-y-1.5">
        {codes.map((code) => (
          <li
            key={code}
            className="font-mono text-meta tabular text-foreground"
          >
            {code}
          </li>
        ))}
      </ul>

      <div className="mt-4 flex flex-wrap gap-2">
        <Button type="button" variant="outline" size="sm" onClick={copy}>
          {copied ? <Check /> : <Copy />}
          {copied ? "Copied" : "Copy"}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={download}>
          <Download />
          Download
        </Button>
      </div>
    </div>
  );
}
