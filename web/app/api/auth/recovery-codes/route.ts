import { callCoreApi } from "@/lib/core-api";
import { handleRoute } from "@/lib/route-helpers";

/** How many codes are left. Never the codes themselves. */
export async function GET(request: Request) {
  return handleRoute(request, () => callCoreApi("/auth/recovery-codes"));
}

/**
 * Mints a fresh set, invalidating the old one.
 *
 * The only response in the system that carries codes in plaintext, and the
 * only chance the reader gets to save them.
 */
export async function POST(request: Request) {
  return handleRoute(request, () =>
    callCoreApi("/auth/recovery-codes", { method: "POST" }),
  );
}
