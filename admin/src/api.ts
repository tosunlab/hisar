import { reactive } from "vue";
import { bridge, type SignUpResult } from "./bridge/bridge";

export interface AdminSession {
  token: string;
  accountId: string;
  deviceId: string;
  name: string;
}

export interface AdminServer {
  publicUrl: string;
}

export type CheckStatus = "ok" | "warning" | "failed";

export interface InstanceCheck {
  id: string;
  title: string;
  status: CheckStatus;
  message: string;
}

export interface AdminHealth {
  version: string;
  // latestVersion is empty unless a newer server release exists.
  latestVersion: string;
  database: { ok: boolean; migrations: { name: string; appliedAt: string | null }[] };
  counts: { accounts: number; suspendedAccounts: number; vaults: number; items: number; shareLinks: number; pendingInvites: number };
  storageBytes: number;
  checks: InstanceCheck[];
}

export const notAdminMessage = "This account is not an admin on the Hisar server.";

const sessionStorageKey = "hisar.admin.session";

const storedSession = sessionStorage.getItem(sessionStorageKey);

export const auth = reactive<{ session: AdminSession | null }>({
  session: storedSession ? (JSON.parse(storedSession) as AdminSession) : null,
});

function saveSession(session: AdminSession | null): void {
  if (session) {
    sessionStorage.setItem(sessionStorageKey, JSON.stringify(session));
  } else {
    sessionStorage.removeItem(sessionStorageKey);
  }
  auth.session = session;
}

export class APIError extends Error {
  constructor(
    readonly status: number,
    message: string,
    // isDeletionPlanChanged marks the 409 that refused an account deletion whose reviewed plan changed; every other
    // deletion conflict keeps its own reason.
    readonly isDeletionPlanChanged = false,
  ) {
    super(message);
  }
}

// send answers the server's response when it succeeds and throws APIError with the server's message otherwise.
export async function send(method: string, path: string, { body, token = auth.session?.token }: { body?: unknown; token?: string } = {}): Promise<Response> {
  const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  const response = await fetch(`/api/v1${path}`, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  if (response.status === 401 && token === auth.session?.token) {
    saveSession(null);
  }
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string; deletionPlanChanged?: boolean };
    throw new APIError(response.status, body.error ?? `The Hisar server answered ${response.status}.`, body.deletionPlanChanged === true);
  }
  return response;
}

export async function request<T>(method: string, path: string, options: { body?: unknown; token?: string } = {}): Promise<T> {
  const response = await send(method, path, options);
  return (response.status === 204 ? undefined : await response.json()) as T;
}

function revokeDevice(deviceId: string, token?: string): Promise<void> {
  return request("DELETE", `/devices/${encodeURIComponent(deviceId)}`, { token });
}

async function revokeDeviceIgnoringFailure(deviceId: string, token: string): Promise<void> {
  await revokeDevice(deviceId, token).catch(() => undefined);
}

export async function signIn(email: string, secretKey: string, password: string): Promise<void> {
  const signedIn = await bridge.signIn(email, secretKey, password);
  if (signedIn.role !== "admin") {
    await revokeDeviceIgnoringFailure(signedIn.deviceId, signedIn.token);
    throw new Error(notAdminMessage);
  }
  saveSession({ token: signedIn.token, accountId: signedIn.accountId, deviceId: signedIn.deviceId, name: signedIn.name });
}

// signUp creates the first admin with the setup invite and signs the dashboard in as that admin.
export async function signUp(inviteToken: string, email: string, name: string, password: string): Promise<SignUpResult> {
  const signedUp = await bridge.signUp(inviteToken, email, name, password);
  saveSession({ token: signedUp.token, accountId: signedUp.accountId, deviceId: signedUp.deviceId, name: name.trim() });
  return signedUp;
}

export async function signOut(): Promise<void> {
  if (auth.session) {
    await revokeDevice(auth.session.deviceId);
  }
  saveSession(null);
}

export function getServer(): Promise<AdminServer> {
  return request("GET", "/admin/server");
}

export function getHealth(): Promise<AdminHealth> {
  return request("GET", "/admin/health");
}

export function confirmSecretsBackup(): Promise<InstanceCheck> {
  return request("POST", "/admin/health/secrets-backup");
}

export function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
