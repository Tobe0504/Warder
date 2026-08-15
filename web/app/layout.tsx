import type { Metadata, Viewport } from "next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";

import { ToastProvider } from "@/components/ui/toast";
import { publicOrigin } from "@/lib/env";

import "./globals.css";

export const metadata: Metadata = {
  // Absolute URLs are built from this. Without it the social card would point
  // at a relative path, which no scraper can fetch.
  metadataBase: new URL(publicOrigin()),

  title: {
    default: "Warder",
    // Pages set their own title; this appends the product to it, so a shared
    // documentation link reads "Threat model — Warder" in a browser tab.
    template: "%s — Warder",
  },
  description: "Use credentials without seeing them.",
  applicationName: "Warder",

  // Search engines have no business here, and a dashboard that indexes itself
  // is a dashboard leaking its own structure. The public route group opts back
  // in for the landing page and the documentation.
  robots: { index: false, follow: false },

  icons: {
    icon: [
      { url: "/favicon-16.png", sizes: "16x16", type: "image/png" },
      { url: "/favicon-32.png", sizes: "32x32", type: "image/png" },
      { url: "/favicon-48.png", sizes: "48x48", type: "image/png" },
      { url: "/icon-192.png", sizes: "192x192", type: "image/png" },
      { url: "/icon-512.png", sizes: "512x512", type: "image/png" },
    ],
    apple: [
      { url: "/apple-touch-icon-180.png", sizes: "180x180", type: "image/png" },
    ],
  },

  manifest: "/manifest.webmanifest",

  /*
   * The social card.
   *
   * Set at the root so that every link into this application unfurls with the
   * lockup rather than with whatever a scraper decides to pull off the page —
   * which, on a dashboard, would be whatever text happened to be on screen.
   */
  openGraph: {
    type: "website",
    siteName: "Warder",
    title: "Warder",
    description: "Use credentials without seeing them.",
    url: "/",
    images: [
      {
        url: "/warder-og-1200x630.png",
        width: 1200,
        height: 630,
        alt: "Warder",
      },
    ],
  },

  twitter: {
    card: "summary_large_image",
    title: "Warder",
    description: "Use credentials without seeing them.",
    images: ["/warder-og-1200x630.png"],
  },
};

/**
 * Browser chrome colour, matched to the page background in each scheme so the
 * status bar on a phone does not sit as a pale strip above a black interface.
 */
export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#fcfcfc" },
    { media: "(prefers-color-scheme: dark)", color: "#000000" },
  ],
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className={`${GeistSans.variable} ${GeistMono.variable}`}>
      <body className="min-h-dvh antialiased">
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  );
}
