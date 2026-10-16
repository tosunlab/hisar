export interface FakeLogin {
  vaultID: string;
  vaultName: string;
  itemID: string;
  title: string;
  username: string;
  password: string;
  url: string;
  oneTimePassword?: string;
}

export interface FakeAddress {
  vaultID: string;
  vaultName: string;
  itemID: string;
  title: string;
  source: "address" | "identity";
  street: string;
  city: string;
  region: string;
  postalCode: string;
  country: string;
}

export interface FakeHostState {
  state: "signed-out" | "locked" | "unlocked";
  email: string;
  isOffline: boolean;
  isDesktopUnavailable: boolean;
  unlockDelayMilliseconds: number;
  unlockAt: number | null;
  pairingCode: string | null;
  pendingPairing: { instanceID: string; browser: string; code: string; expiresAt: number; attemptCount: number } | null;
  allPairing: { instanceID: string; browser: string; tokenHash: string; pairedAt: number }[];
  exactHostOnly: boolean;
  allLogin: FakeLogin[];
  features?: string[];
  allAddress?: FakeAddress[];
  allAddressRequest?: string[];
  allCodeRequest?: { itemID: string; url: string }[];
  allPasskey?: FakePasskey[];
  updateRequired?: { target: "app" | "extension"; currentVersion: string; minimumVersion: string; updateURL: string } | null;
}

export interface FakePasskey {
  credentialId: string;
  relyingPartyId: string;
  userHandle: string;
  userName: string;
  userDisplayName: string;
  privateKey: string;
  vaultID: string;
  itemID: string;
  createdAt: number;
}

export const protocolVersion: number;
export function createInitialState(): FakeHostState;
export function readState(stateFile: string): FakeHostState;
export function writeState(stateFile: string, state: FakeHostState): void;
export function doesUrlMatch(itemUrl: string, pageUrl: string, exactHostOnly: boolean): boolean;
export function handleRequest(state: FakeHostState, request: Record<string, unknown>, now?: number, log?: (line: string) => void): unknown;
export function respond(
  state: FakeHostState,
  request: Record<string, unknown>,
  now?: number,
  log?: (line: string) => void,
): { id: string; ok: boolean; result?: unknown; error?: { code: string; message: string } };
export function generateOneTimeCode(setupKey: string, now: number): { code: string; expiresAt: string; period: number };
export function encodeFrame(message: unknown): Buffer;
export function createFrameReader(onMessage: (message: unknown) => void): (chunk: Buffer) => void;
