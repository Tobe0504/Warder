"use client";

/**
 * The last-resort error boundary, for failures in the root layout itself.
 *
 * It replaces the whole document, so it carries its own html and body, and it
 * uses inline styles rather than Tailwind — if the root layout failed, the
 * stylesheet may be exactly what did not load.
 *
 * It shows no error detail. A root-layout failure is most often a configuration
 * problem, and configuration here contains a credential.
 */
export default function GlobalError({ reset }: { error: Error; reset: () => void }) {
  return (
    <html lang="en">
      <body
        style={{
          fontFamily: "ui-sans-serif, system-ui, sans-serif",
          display: "flex",
          minHeight: "100dvh",
          alignItems: "center",
          justifyContent: "center",
          margin: 0,
          padding: "1rem",
          background: "#fafafa",
          color: "#18181b",
        }}
      >
        <div style={{ maxWidth: "24rem", textAlign: "center" }}>
          {/*
            A plain img, not next/image. This boundary exists for the case where
            the root layout itself failed, so it depends on nothing but the
            static file — and the page is painted on a light background, so the
            ink artwork is the right one regardless of colour scheme.
          */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src="/warder-mark-1024-ink.png"
            alt="Warder"
            width={26}
            height={26}
            style={{ display: "block", margin: "0 auto 0.75rem" }}
          />
          <h1 style={{ fontSize: "0.875rem", fontWeight: 500, margin: 0 }}>
            Warder could not start
          </h1>
          <p style={{ fontSize: "0.75rem", lineHeight: 1.6, color: "#71717a", marginTop: "0.5rem" }}>
            This usually means <code>WARDER_URL</code> is missing or malformed. The server console
            has the details.
          </p>
          <p style={{ fontSize: "0.75rem", color: "#71717a", marginTop: "0.5rem" }}>
            Generate a working configuration with{" "}
            <code style={{ fontFamily: "ui-monospace, monospace" }}>
              go run ./cmd/warder-api init
            </code>
          </p>
          <button
            onClick={reset}
            style={{
              marginTop: "1rem",
              padding: "0.375rem 0.75rem",
              fontSize: "0.75rem",
              borderRadius: "0.375rem",
              border: "1px solid #d4d4d8",
              background: "#fff",
              cursor: "pointer",
            }}
          >
            Try again
          </button>
        </div>
      </body>
    </html>
  );
}
