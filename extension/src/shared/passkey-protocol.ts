import type { PasskeyPRFOutputJSON, SerializedPRFInput, UserVerificationPolicy } from "./passkey";

export interface DesktopPasskeyParameters {
  userVerification?: UserVerificationPolicy;
  ceremonyId?: string;
  origin: string;
  topOrigin?: string;
  isCrossOrigin?: boolean;
  relyingPartyId?: string;
  challenge?: string;
  userHandle?: string;
  userName?: string;
  userDisplayName?: string;
  allowCredentials?: string[];
  excludeCredentials?: string[];
  credentialId?: string;
  vaultID?: string;
  itemID?: string;
  prf?: SerializedPRFInput;
  allAcceptedCredentialIds?: string[];
}

export interface DesktopPasskeySummary {
  credentialId: string;
  vaultID: string;
  itemID: string;
  title: string;
  userName: string;
  userDisplayName: string;
}

export interface DesktopPasskeyCreation {
  credentialId: string;
  clientDataJSON: string;
  attestationObject: string;
  authenticatorData: string;
  publicKey: string;
  publicKeyAlgorithm: number;
  vaultID: string;
  itemID: string;
  prf?: PasskeyPRFOutputJSON;
}

export interface DesktopPasskeyAssertion {
  credentialId: string;
  clientDataJSON: string;
  authenticatorData: string;
  signature: string;
  userHandle: string;
  prf?: PasskeyPRFOutputJSON;
}

export const allPasskeySignalType = ["passkeys.signalUnknownCredential", "passkeys.signalAllAcceptedCredentials", "passkeys.signalCurrentUserDetails"] as const;

export type PasskeySignalType = (typeof allPasskeySignalType)[number];

declare module "./protocol" {
  interface RequestMap {
    "passkeys.list": { parameters: { passkey: DesktopPasskeyParameters }; result: { passkeys: DesktopPasskeySummary[] } };
    "passkeys.prepare": { parameters: { passkey: DesktopPasskeyParameters }; result: { isExcluded: boolean; logins: LoginSummary[] } };
    "passkeys.create": { parameters: { passkey: DesktopPasskeyParameters }; result: DesktopPasskeyCreation };
    "passkeys.get": { parameters: { passkey: DesktopPasskeyParameters }; result: DesktopPasskeyAssertion };
    "passkeys.cancel": { parameters: { passkey: DesktopPasskeyParameters }; result: null };
    "passkeys.signalUnknownCredential": { parameters: { passkey: DesktopPasskeyParameters }; result: { changedPasskeyCount: number } };
    "passkeys.signalAllAcceptedCredentials": { parameters: { passkey: DesktopPasskeyParameters }; result: { changedPasskeyCount: number } };
    "passkeys.signalCurrentUserDetails": { parameters: { passkey: DesktopPasskeyParameters }; result: { changedPasskeyCount: number } };
  }
}
