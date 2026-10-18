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

// FakeItem is an item other than a login, which items.list and items.get show; fields carry every value, and items.get
// leaves out a secret's as the app does.
export interface FakeItem {
  vaultID: string;
  vaultName: string;
  itemID: string;
  category: string;
  title: string;
  subtitle: string;
  url?: string;
  fields: { id: string; label: string; kind: "text" | "secret" | "url" | "notes" | "one-time-code"; value?: string }[];
}

export interface FakeCard {
  vaultID: string;
  vaultName: string;
  itemID: string;
  title: string;
  brand: string;
  last4: string;
  expiry: string;
  cardholder: string;
  number: string;
  expiryMonth: string;
  expiryYear: string;
  securityCode: string;
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
  masterPassword?: string;
  allAddress?: FakeAddress[];
  allItem?: FakeItem[];
  allCard?: FakeCard[];
  allCardRequest?: string[];
  allFavoriteItemID?: string[];
  allRevealRequest?: string[];
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
