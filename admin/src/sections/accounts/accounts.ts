import { auth, request } from "../../api";
import { bridge, type ReauthenticationProof } from "../../bridge/bridge";

export type AccountRole = "admin" | "member";

export type AccountState = "active" | "suspended";

// AdminAccount is an account as GET /api/v1/admin/accounts lists it; lastSeenAt is null for an account without devices.
export interface AdminAccount {
  id: string;
  email: string;
  name: string;
  role: AccountRole;
  state: AccountState;
  createdAt: string;
  lastSeenAt: string | null;
  deviceCount: number;
  vaultCount: number;
}

export interface AdminAccountPage {
  accounts: AdminAccount[];
  nextCursor?: string;
}

export interface AdminDevice {
  id: string;
  name: string;
  platform: string;
  createdAt: string;
  lastSeenAt: string;
  revokedAt: string | null;
}

export interface AdminAccountDetail {
  account: AdminAccount;
  devices: AdminDevice[];
}

export const accountsPath = "/admin/accounts";

export const roleLabels: Record<AccountRole, string> = { admin: "Admin", member: "Member" };

export const stateLabels: Record<AccountState, string> = { active: "Active", suspended: "Suspended" };

// describeDevice says whether a device is signed in. A suspended account's devices keep their sessions but cannot reach
// the server until it is reactivated, so they show Suspended rather than Active.
export function describeDevice(device: AdminDevice, accountState: AccountState): string {
  if (device.revokedAt) {
    return `Signed out ${formatDate(device.revokedAt)}`;
  }
  const activity = formatActivity(device.lastSeenAt);
  return accountState === "suspended" ? `Suspended · last activity ${activity}` : `Active ${activity}`;
}

// formatDeviceCount is the Devices column: the signed-in devices, which a suspension keeps but stops.
export function formatDeviceCount(account: AdminAccount): string {
  return account.state === "suspended" && account.deviceCount > 0 ? `${account.deviceCount} suspended` : String(account.deviceCount);
}

export const ownAccountMessage = "Use Settings → Account in the Hisar app for your own account.";

function accountPath(accountId: string): string {
  return `/admin/accounts/${encodeURIComponent(accountId)}`;
}

export function listAccounts(query: string, cursor = ""): Promise<AdminAccountPage> {
  const parameters = new URLSearchParams();
  if (query) {
    parameters.set("query", query);
  }
  if (cursor) {
    parameters.set("cursor", cursor);
  }
  const search = parameters.size > 0 ? `?${parameters}` : "";
  return request("GET", `/admin/accounts${search}`);
}

export function getAccount(accountId: string): Promise<AdminAccountDetail> {
  return request("GET", accountPath(accountId));
}

export function suspendAccount(accountId: string): Promise<void> {
  return request("POST", `${accountPath(accountId)}/suspend`);
}

export function reactivateAccount(accountId: string): Promise<void> {
  return request("POST", `${accountPath(accountId)}/reactivate`);
}

export function signOutAccount(accountId: string): Promise<void> {
  return request("POST", `${accountPath(accountId)}/sign-out`);
}

export function revokeAccountDevice(accountId: string, deviceId: string): Promise<void> {
  return request("DELETE", `${accountPath(accountId)}/devices/${encodeURIComponent(deviceId)}`);
}

export function changeAccountRole(accountId: string, role: AccountRole, proof: ReauthenticationProof): Promise<void> {
  return request("PUT", `${accountPath(accountId)}/role`, { body: { role, proof } });
}

export function deleteAccount(accountId: string, email: string, proof: ReauthenticationProof): Promise<void> {
  return request("POST", `${accountPath(accountId)}/delete`, { body: { email, proof } });
}

// proveAdmin proves the signed-in admin's master password to the server, for a change that asks for it again.
export async function proveAdmin(secretKey: string, password: string): Promise<ReauthenticationProof> {
  const session = auth.session;
  if (!session) {
    throw new Error("Sign in again.");
  }
  const { email } = await request<{ email: string }>("GET", "/account");
  return bridge.reauthenticate(session.token, email, secretKey, password);
}

// readAccountId returns the account ID in a path under /admin/accounts/, or "" for the list. A segment that is not valid
// percent-encoding is taken as typed, so it names no account.
export function readAccountId(pathname: string): string {
  const rest = pathname.replace(/\/+$/, "").slice(accountsPath.length + 1);
  if (!pathname.startsWith(`${accountsPath}/`) || !rest || rest.includes("/")) {
    return "";
  }
  try {
    return decodeURIComponent(rest);
  } catch {
    return rest;
  }
}

const dateFormat = new Intl.DateTimeFormat("en", { dateStyle: "medium" });
const relativeFormat = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

export function formatDate(timestamp: string): string {
  return dateFormat.format(new Date(timestamp));
}

// formatActivity writes when an account or device was last active: "5 minutes ago", "yesterday", or "Never".
export function formatActivity(timestamp: string | null, now = Date.now()): string {
  if (!timestamp) {
    return "Never";
  }
  const seconds = Math.round((new Date(timestamp).getTime() - now) / 1000);
  for (const [unit, size] of [
    ["day", 86_400],
    ["hour", 3_600],
    ["minute", 60],
  ] as const) {
    if (Math.abs(seconds) >= size) {
      return relativeFormat.format(Math.round(seconds / size), unit);
    }
  }
  return "Just now";
}
