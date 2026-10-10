import type { FakeHostState, FakeLogin } from "./fake-native-host.mjs";

export const hisarAAGUID: string;
export class PasskeyError extends Error {
  code: string;
}
export function derivePRFSecret(privateKeyDERBase64: string): Buffer;
export function evaluatePRF(secret: Buffer, input: Buffer): Buffer;
export function resolveRelyingPartyID(origin: string, requestedID?: string): string;
export function handlePasskeyRequest(
  state: FakeHostState,
  request: Record<string, unknown>,
  now: number,
  findAllMatchingLogin: (state: FakeHostState, url: string) => FakeLogin[],
): unknown;
