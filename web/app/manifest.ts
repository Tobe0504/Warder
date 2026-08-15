import type { MetadataRoute } from "next";

/**
 * The web app manifest.
 *
 * Served at /manifest.webmanifest. It exists so that an installed shortcut on
 * a phone or desktop carries the mark and the product's colours rather than a
 * screenshot of whatever page was open.
 *
 * `display: browser` on purpose. A standalone window hides the address bar,
 * and the address bar is where someone checks that the origin asking for their
 * password is the one they expect — which matters more on a credential tool
 * than an app-like frame does.
 */
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Warder",
    short_name: "Warder",
    description: "Use credentials without seeing them.",
    start_url: "/",
    display: "browser",
    background_color: "#000000",
    theme_color: "#000000",
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
      // `maskable` lets Android crop to whatever shape the launcher uses
      // without clipping the mark, since the artwork already sits inside a
      // padded rounded square.
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
