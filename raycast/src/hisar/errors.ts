/** The codes Hisar answers with (core/cliprotocol); the wire spec checks them against the contract. */
export const allAppErrorCode = [
  "invalid-request",
  "integration-disabled",
  "signed-out",
  "locked",
  "denied",
  "offline",
  "not-found",
  "internal",
  "update-required",
] as const;

/** The codes of failures the extension meets before Hisar can answer. */
export const allClientErrorCode = ["app-not-installed", "app-not-running", "connection-lost", "timeout"] as const;

export const allErrorCode = [...allAppErrorCode, ...allClientErrorCode] as const;

export type AppErrorCode = (typeof allAppErrorCode)[number];
export type ErrorCode = (typeof allErrorCode)[number];

export class HisarError extends Error {
  readonly code: ErrorCode;

  constructor(code: ErrorCode, message: string) {
    super(message);
    this.name = "HisarError";
    this.code = code;
  }
}

export function isAppErrorCode(value: unknown): value is AppErrorCode {
  return typeof value === "string" && (allAppErrorCode as readonly string[]).includes(value);
}

export function toHisarError(error: unknown): HisarError {
  if (error instanceof HisarError) return error;
  if (error instanceof Error) return new HisarError("internal", error.message);
  return new HisarError("internal", String(error));
}
