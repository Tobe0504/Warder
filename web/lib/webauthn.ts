/**
 * The browser half of a WebAuthn ceremony.
 *
 * The whole file exists because of one mismatch: the server speaks base64url
 * strings and `navigator.credentials` speaks ArrayBuffers, in both directions.
 * Getting a single field wrong produces a credential that is accepted by the
 * authenticator and then rejected by the server with no useful explanation, so
 * the conversion lives in one place rather than at each call site.
 */

/** Reports whether this browser can do WebAuthn at all. */
export function passkeysSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.PublicKeyCredential === "function"
  );
}

function fromBase64Url(value: string): ArrayBuffer {
  // base64url omits padding and swaps two characters; atob wants neither.
  const padded = value.replace(/-/g, "+").replace(/_/g, "/");
  const binary = atob(padded.padEnd(Math.ceil(padded.length / 4) * 4, "="));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

function toBase64Url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

/**
 * The options as they arrive over the wire.
 *
 * Deliberately not the DOM's own option types: those declare BufferSource for
 * every field the server sends as a base64url string, so describing the shape
 * honestly is what lets the conversions below be checked rather than asserted.
 */
type WireCredential = { id: string; type: string; transports?: string[] };

type CreationOptions = {
  publicKey: Omit<
    PublicKeyCredentialCreationOptions,
    "challenge" | "user" | "excludeCredentials"
  > & {
    challenge: string;
    user: { id: string; name: string; displayName: string };
    excludeCredentials?: WireCredential[];
  };
};

type RequestOptions = {
  publicKey: Omit<
    PublicKeyCredentialRequestOptions,
    "challenge" | "allowCredentials"
  > & {
    challenge: string;
    allowCredentials?: WireCredential[];
  };
};

/**
 * Registers a new credential.
 *
 * `options` is the server's response passed through untouched; only the fields
 * the API needs as binary are converted.
 */
export async function createCredential(
  options: CreationOptions,
): Promise<Record<string, unknown>> {
  const publicKey = options.publicKey;

  const credential = (await navigator.credentials.create({
    publicKey: {
      ...publicKey,
      challenge: fromBase64Url(publicKey.challenge),
      user: {
        ...publicKey.user,
        id: fromBase64Url(publicKey.user.id),
      },
      excludeCredentials: (publicKey.excludeCredentials ?? []).map((c) => ({
        id: fromBase64Url(c.id),
        type: "public-key" as const,
        transports: c.transports as AuthenticatorTransport[] | undefined,
      })),
    },
  })) as PublicKeyCredential | null;

  if (!credential) {
    throw new Error("No passkey was created.");
  }

  const response = credential.response as AuthenticatorAttestationResponse;

  return {
    id: credential.id,
    rawId: toBase64Url(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: toBase64Url(response.clientDataJSON),
      attestationObject: toBase64Url(response.attestationObject),
    },
    // The server reads this to record how the key can be reached next time.
    transports:
      typeof response.getTransports === "function"
        ? response.getTransports()
        : [],
  };
}

/**
 * Answers a challenge with an existing credential.
 *
 * With no `allowCredentials`, the browser offers whatever discoverable keys it
 * holds for this domain, which is what makes a sign-in with no address typed
 * possible.
 */
export async function getCredential(
  options: RequestOptions,
): Promise<Record<string, unknown>> {
  const publicKey = options.publicKey;

  const credential = (await navigator.credentials.get({
    publicKey: {
      ...publicKey,
      challenge: fromBase64Url(publicKey.challenge),
      allowCredentials: (publicKey.allowCredentials ?? []).map((c) => ({
        id: fromBase64Url(c.id),
        type: "public-key" as const,
        transports: c.transports as AuthenticatorTransport[] | undefined,
      })),
    },
  })) as PublicKeyCredential | null;

  if (!credential) {
    throw new Error("No passkey was used.");
  }

  const response = credential.response as AuthenticatorAssertionResponse;

  return {
    id: credential.id,
    rawId: toBase64Url(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: toBase64Url(response.clientDataJSON),
      authenticatorData: toBase64Url(response.authenticatorData),
      signature: toBase64Url(response.signature),
      userHandle: response.userHandle
        ? toBase64Url(response.userHandle)
        : null,
    },
  };
}

/**
 * Turns a ceremony failure into something worth reading.
 *
 * The DOM names are accurate and useless to a person: NotAllowedError is what
 * both "you cancelled" and "it timed out" produce.
 */
export function describePasskeyError(error: unknown): string {
  if (error instanceof DOMException) {
    switch (error.name) {
      case "NotAllowedError":
        return "No passkey was used. The prompt was dismissed or timed out.";
      case "InvalidStateError":
        return "That passkey is already registered on this account.";
      case "SecurityError":
        return "This page's address does not match where the passkey was registered.";
      case "AbortError":
        return "The passkey prompt was cancelled.";
    }
  }
  return error instanceof Error ? error.message : "That passkey did not work.";
}
