import { decodeBase64URL, encodeBase64URL, type PasskeyPRFOutputJSON, type SerializedPRFInput, type SerializedPRFValues } from "../../shared/passkey";

interface PRFValues {
  first: BufferSource;
  second?: BufferSource;
}

interface PRFExtensionInput {
  eval?: PRFValues;
  evalByCredential?: Record<string, PRFValues>;
}

function toBytes(source: unknown): Uint8Array {
  if (source instanceof ArrayBuffer) return new Uint8Array(source.slice(0));
  if (ArrayBuffer.isView(source)) return new Uint8Array(source.buffer.slice(source.byteOffset, source.byteOffset + source.byteLength));
  throw new TypeError("expected a BufferSource");
}

function readPRFExtension(extensions: unknown): PRFExtensionInput | undefined {
  const prf = (extensions as { prf?: unknown } | undefined)?.prf;
  return prf === undefined || prf === null ? undefined : (prf as PRFExtensionInput);
}

function normalizeBase64URL(text: string): string | undefined {
  try {
    return encodeBase64URL(decodeBase64URL(text));
  } catch {
    return undefined;
  }
}

// The client checks of the PRF extension (WebAuthn, prf extension, client extension processing).
export function findPRFInputError(
  kind: "create" | "get",
  publicKey: { extensions?: unknown; allowCredentials?: Iterable<PublicKeyCredentialDescriptor> },
  createException: (message: string, name: string) => Error,
): Error | undefined {
  const prf = readPRFExtension(publicKey.extensions);
  if (!prf) return undefined;
  const allEvalByCredentialID = prf.evalByCredential === undefined ? [] : Object.keys(prf.evalByCredential);
  if (kind === "create") {
    return prf.evalByCredential !== undefined ? createException("evalByCredential is not supported when creating a credential", "NotSupportedError") : undefined;
  }
  if (allEvalByCredentialID.length === 0) return undefined;
  const allAllowedID = Array.from(publicKey.allowCredentials ?? [], (descriptor) => encodeBase64URL(toBytes(descriptor.id)));
  if (allAllowedID.length === 0) return createException("evalByCredential needs allowCredentials", "NotSupportedError");
  for (const credentialID of allEvalByCredentialID) {
    const normalizedID = normalizeBase64URL(credentialID);
    if (!normalizedID || !allAllowedID.includes(normalizedID)) {
      return createException("an evalByCredential key is not the ID of an allowCredentials entry", "SyntaxError");
    }
  }
  return undefined;
}

function serializePRFValues(values: PRFValues): SerializedPRFValues {
  return {
    first: encodeBase64URL(toBytes(values.first)),
    ...(values.second === undefined ? {} : { second: encodeBase64URL(toBytes(values.second)) }),
  };
}

export function serializePRFInput(extensions: unknown): SerializedPRFInput | undefined {
  const prf = readPRFExtension(extensions);
  if (!prf) return undefined;
  const serialized: SerializedPRFInput = {};
  if (prf.eval !== undefined && prf.eval !== null) serialized.eval = serializePRFValues(prf.eval);
  if (prf.evalByCredential !== undefined && prf.evalByCredential !== null) {
    serialized.evalByCredential = Object.fromEntries(Object.entries(prf.evalByCredential).map(([credentialID, values]) => [encodeBase64URL(decodeBase64URL(credentialID)), serializePRFValues(values)]));
  }
  return serialized;
}

function toArrayBuffer(text: string): ArrayBuffer {
  const bytes = decodeBase64URL(text);
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

export function buildPRFExtensionResult(output: PasskeyPRFOutputJSON): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  if (output.enabled !== undefined) result.enabled = output.enabled;
  if (output.results) {
    result.results = {
      first: toArrayBuffer(output.results.first),
      ...(output.results.second === undefined ? {} : { second: toArrayBuffer(output.results.second) }),
    };
  }
  return result;
}

export function buildPRFExtensionResultJSON(output: PasskeyPRFOutputJSON): Record<string, unknown> {
  return {
    ...(output.enabled === undefined ? {} : { enabled: output.enabled }),
    ...(output.results ? { results: { ...output.results } } : {}),
  };
}
