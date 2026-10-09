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
  "internal",
] as const;

export type ErrorCode = (typeof allErrorCode)[number];

export type DesktopState = "signed-out" | "locked" | "unlocked";

export interface LoginSummary {
  vaultID: string;
  itemID: string;
  title: string;
  username: string;
  vaultName: string;
}

export interface HelloResult {
  protocolVersion: number;
  isPaired: boolean;
  state: DesktopState;
}

export interface StatusResult {
  state: DesktopState;
  email: string;
  isOffline: boolean;
}

export interface Credentials {
  username: string;
  password: string;
}

export type CheckStatus = "new" | "changed" | "unchanged";

export interface CheckResult {
  status: CheckStatus;
  vaultID?: string;
  itemID?: string;
  title?: string;
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
  "logins.match": { parameters: { url: string }; result: { logins: LoginSummary[] } };
  "logins.fill": { parameters: { vaultID: string; itemID: string; url: string }; result: Credentials };
  "logins.check": { parameters: { url: string; username: string; password: string }; result: CheckResult };
  "logins.save": {
    parameters: { url: string; username: string; password: string; vaultID?: string; itemID?: string };
    result: { vaultID: string; itemID: string };
  };
  "password.generate": { parameters: GenerateOptions; result: { password: string } };
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
