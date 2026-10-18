import { request, send } from "../../api";

// AuditEvent is one row of GET /api/v1/admin/audit; accountEmail is null for a deleted account.
export interface AuditEvent {
  id: number;
  accountId: string | null;
  accountEmail: string | null;
  deviceId: string | null;
  type: string;
  detail: unknown;
  ip: string;
  createdAt: string;
}

export interface AuditPage {
  events: AuditEvent[];
  // nextCursor is empty on the last page.
  nextCursor: string;
}

export interface AuditTypeGroup {
  id: string;
  label: string;
  types: { type: string; label: string }[];
}

export interface AuditFilters {
  account: string;
  kind: "" | `type:${string}` | `group:${string}`;
  // from and to are datetime-local values, in the browser's time zone.
  from: string;
  to: string;
}

export function buildQuery(filters: AuditFilters, cursor = ""): string {
  const query = new URLSearchParams();
  const account = filters.account.trim();
  if (account) {
    query.set("account", account);
  }
  const [kind, value] = filters.kind.split(":", 2);
  if (kind === "type" || kind === "group") {
    query.set(kind, value);
  }
  for (const name of ["from", "to"] as const) {
    if (filters[name]) {
      query.set(name, new Date(filters[name]).toISOString());
    }
  }
  if (cursor) {
    query.set("cursor", cursor);
  }
  const text = query.toString();
  return text ? `?${text}` : "";
}

export function describeDetail(detail: unknown): string {
  if (detail === null || detail === undefined) {
    return "";
  }
  if (typeof detail !== "object" || Array.isArray(detail)) {
    return JSON.stringify(detail);
  }
  return Object.entries(detail)
    .map(([name, value]) => `${name}: ${Array.isArray(value) ? value.join(", ") : typeof value === "object" ? JSON.stringify(value) : String(value)}`)
    .join(" · ");
}

export async function getAuditTypes(): Promise<AuditTypeGroup[]> {
  return (await request<{ groups: AuditTypeGroup[] }>("GET", "/admin/audit/types")).groups;
}

export function listAuditEvents(filters: AuditFilters, cursor = ""): Promise<AuditPage> {
  return request("GET", `/admin/audit${buildQuery(filters, cursor)}`);
}

export async function exportAuditEvents(filters: AuditFilters): Promise<void> {
  const response = await send("GET", `/admin/audit.csv${buildQuery(filters)}`);
  const fileName = /filename="([^"]+)"/.exec(response.headers.get("Content-Disposition") ?? "")?.[1] ?? "hisar-audit.csv";
  const link = document.createElement("a");
  link.href = URL.createObjectURL(await response.blob());
  link.download = fileName;
  link.click();
  setTimeout(() => URL.revokeObjectURL(link.href), 0);
}
