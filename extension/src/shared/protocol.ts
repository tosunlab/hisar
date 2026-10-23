export const nativeHostName = "com.tryhisar.hisar";
export const protocolVersion = 1;

export const allErrorCode = [
  "invalid-request",
  "not-paired",
  "pairing-failed",
  "signed-out",
  "locked",
  "offline",
  "not-found",
  "no-match",
  "desktop-unavailable",
  "update-required",
  "wrong-password",
  "passkey-verification-failed",
  "internal",
] as const;

export type ErrorCode = (typeof allErrorCode)[number];

export type DesktopState = "signed-out" | "locked" | "unlocked";

export interface LoginSummary {
  isPersonal?: boolean;
  vaultID: string;
  itemID: string;
  title: string;
  username: string;
  vaultName: string;
  hasOneTimePassword?: boolean;
}

export interface UpdateRequirement {
  target: "app" | "extension";
  currentVersion: string;
  minimumVersion: string;
  updateURL: string;
}

// HelloResult: features lists what the app answers beyond the first version of the protocol; an older app sends none.
export interface HelloResult {
  protocolVersion: number;
  isPaired: boolean;
  state: DesktopState;
  updateRequired?: UpdateRequirement;
  features?: string[];
}

// featureAddresses: the app answers addresses.list and addresses.fill.
export const featureAddresses = "addresses";

// featureVault: the app answers items.list, items.get, items.reveal, items.code, items.icon and items.favorite, so the
// popup browses every vault.
export const featureVault = "vault";

// featureCards: the app answers cards.list and cards.fill.
export const featureCards = "cards";

// CardSummary is a card offered for filling: brand, last four digits and expiry; the rest comes in cards.fill.
export interface CardSummary {
  vaultID: string;
  itemID: string;
  title: string;
  brand: string;
  last4: string;
  expiry: string;
  vaultName: string;
}

// CardValues is the card to fill a payment form with: number holds only digits, expiryMonth two digits and expiryYear four.
export interface CardValues {
  cardholder: string;
  number: string;
  expiryMonth: string;
  expiryYear: string;
  securityCode: string;
  brand: string;
}

// featureIdentities: the app answers identities.list and identities.fill.
export const featureIdentities = "identities";

// IdentitySummary is an identity offered for filling: its summary is only the name and email.
export interface IdentitySummary {
  vaultID: string;
  itemID: string;
  title: string;
  summary: string;
  vaultName: string;
}

// IdentityValues is the identity to fill a form with, and its address when it has one worth filling.
export interface IdentityValues {
  firstName: string;
  middleName: string;
  lastName: string;
  email: string;
  phone: string;
  birthDate: string;
  address?: AddressValues;
}

// featureSaveOptions: the app answers vaults.list, and logins.save takes a title and a vault for a new login.
export const featureSaveOptions = "save-options";
export const featureSaveConfirmation = "save-confirmation";

// featureCreateItems: the app answers items.create.
export const featureCreateItems = "create-items";

// VaultSummary is a vault a new item may be saved into; the personal vault comes first.
export interface VaultSummary {
  vaultID: string;
  name: string;
  isPersonal: boolean;
}

// NewItem is what items.create saves: a login, a secure note, a card or an identity, with that category's fields.
export interface NewItem {
  category: "login" | "secure-note" | "credit-card" | "identity";
  title: string;
  username?: string;
  password?: string;
  url?: string;
  notes?: string;
  cardholder?: string;
  cardNumber?: string;
  expiry?: string;
  securityCode?: string;
  firstName?: string;
  lastName?: string;
  email?: string;
  phone?: string;
}

// featureUnlockPassword: the app answers unlock.password, so the popup unlocks Hisar with the master password.
export const featureUnlockPassword = "unlock-password";

// ItemSummary is one row of the popup's item list: metadata only, never a secret.
export interface ItemSummary {
  vaultID: string;
  itemID: string;
  category: string;
  title: string;
  subtitle: string;
  vaultName: string;
  url?: string;
  isFavorite: boolean;
  hasOneTimePassword: boolean;
}

// ItemFieldKind: a secret's value is left out of items.get and read with items.reveal; a one-time code's comes from
// items.code.
export type ItemFieldKind = "text" | "secret" | "url" | "notes" | "one-time-code";

export interface ItemField {
  id: string;
  label: string;
  kind: ItemFieldKind;
  value?: string;
}

export interface ItemResult extends ItemSummary {
  fields: ItemField[];
}

// AddressSummary is an address offered for filling: its summary is only the city and country; the rest comes in
// addresses.fill.
export interface AddressSummary {
  vaultID: string;
  itemID: string;
  title: string;
  summary: string;
  source: "address" | "identity";
  vaultName: string;
}

// AddressValues is the address to fill a form with; street may hold several lines.
export interface AddressValues {
  street: string;
  city: string;
  region: string;
  postalCode: string;
  country: string;
}

export interface StatusResult {
  state: DesktopState;
  email: string;
  isOffline: boolean;
}

export interface Credentials {
  username: string;
  password: string;
  hasOneTimePassword?: boolean;
}

export interface OneTimeCode {
  code: string;
  expiresAt: string;
  period: number;
}

export type CheckStatus = "new" | "changed" | "unchanged";

export interface CheckResult {
  status: CheckStatus;
  vaultID?: string;
  itemID?: string;
  title?: string;
  saveContext?: string;
  vaultName?: string;
  isPersonal?: boolean;
}

export interface GenerateOptions {
  length?: number;
  digits?: boolean;
  symbols?: boolean;
}

export interface RequestMap {
  hello: { parameters: { instanceID: string; browser: string; extensionVersion: string }; result: HelloResult };
  "pair.start": { parameters: { instanceID: string; browser: string }; result: Record<string, never> };
  "pair.finish": { parameters: { instanceID: string; code: string }; result: { token: string } };
  status: { parameters: Record<string, never>; result: StatusResult };
  unlock: { parameters: Record<string, never>; result: Record<string, never> };
  "unlock.password": { parameters: { password: string }; result: StatusResult };
  "logins.match": { parameters: { url: string }; result: { logins: LoginSummary[] } };
  "logins.fill": { parameters: { vaultID: string; itemID: string; url: string }; result: Credentials };
  "logins.code": { parameters: { vaultID: string; itemID: string; url: string }; result: OneTimeCode };
  // tabID is the tab whose offer this is: Hisar keeps one save context per tab, and the tab's next prepare retires it.
  "logins.prepare": { parameters: { tabID: string; url: string; username: string; password: string }; result: CheckResult };
  "logins.review": { parameters: { saveContext: string }; result: Record<string, never> };
  "logins.save": {
    parameters: { url: string; username: string; password: string; vaultID?: string; itemID?: string; title?: string; saveContext: string };
    result: { vaultID: string; itemID: string };
  };
  "identities.list": { parameters: Record<string, never>; result: { identities: IdentitySummary[] } };
  "identities.fill": { parameters: { vaultID: string; itemID: string }; result: IdentityValues };
  "cards.list": { parameters: Record<string, never>; result: { cards: CardSummary[] } };
  "cards.fill": { parameters: { vaultID: string; itemID: string }; result: CardValues };
  "vaults.list": { parameters: Record<string, never>; result: { vaults: VaultSummary[] } };
  "items.create": { parameters: { vaultID: string; item: NewItem }; result: ItemSummary };
  "items.list": {
    parameters: { query?: string; category?: string; isFavoriteOnly?: boolean };
    result: { items: ItemSummary[] };
  };
  "items.get": { parameters: { vaultID: string; itemID: string }; result: ItemResult };
  "items.reveal": { parameters: { vaultID: string; itemID: string; fieldID: string }; result: { value: string } };
  "items.code": { parameters: { vaultID: string; itemID: string }; result: OneTimeCode };
  "items.icon": { parameters: { url: string }; result: { png?: string } };
  "items.favorite": { parameters: { vaultID: string; itemID: string; isFavorite: boolean }; result: Record<string, never> };
  "password.generate": { parameters: GenerateOptions; result: { password: string } };
  "addresses.list": { parameters: Record<string, never>; result: { addresses: AddressSummary[] } };
  "addresses.fill": { parameters: { vaultID: string; itemID: string }; result: AddressValues };
}

export type RequestType = keyof RequestMap;

export const allUnauthenticatedType: readonly RequestType[] = ["hello", "pair.start", "pair.finish"];

export interface ResponseFrame {
  id: string;
  ok: boolean;
  result?: unknown;
  error?: { code?: string; message?: string };
}

export class HisarError extends Error {
  readonly code: ErrorCode;

  constructor(code: ErrorCode, message?: string) {
    super(message || code);
    this.name = "HisarError";
    this.code = code;
  }
}

export function isErrorCode(value: unknown): value is ErrorCode {
  return typeof value === "string" && (allErrorCode as readonly string[]).includes(value);
}

export function toHisarError(error: unknown): HisarError {
  if (error instanceof HisarError) return error;
  if (error instanceof Error) return new HisarError("internal", error.message);
  return new HisarError("internal", String(error));
}
