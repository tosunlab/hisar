import { request } from "../../api";

export type InviteRole = "member" | "admin";

// AdminInvite is a pending sign-up invite as GET /api/v1/admin/invites returns it; createdBy is null for an invite from
// hisar-server invite.
export interface AdminInvite {
  id: string;
  email: string;
  role: InviteRole;
  createdBy: { accountId: string; name: string; email: string } | null;
  createdAt: string;
  expiresAt: string;
}

// CreatedInvite answers a create or a resend; no other answer holds the link.
export interface CreatedInvite {
  invite: AdminInvite;
  link: string;
  emailed: boolean;
  emailError?: string;
}

export const roleLabels: Readonly<Record<InviteRole, string>> = { member: "Member", admin: "Admin" };

const dateFormat = new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "long", year: "numeric" });

// formatDate writes a date as the invite email and the invite page do: 25 October 2026.
export function formatDate(timestamp: string): string {
  return dateFormat.format(new Date(timestamp));
}

export function describeSender(invite: AdminInvite): string {
  return invite.createdBy ? invite.createdBy.name.trim() || invite.createdBy.email : "hisar-server invite";
}

// describeDelivery says whether the invite went out by email; the admin copies the link when it did not.
export function describeDelivery(created: CreatedInvite): string {
  if (created.emailed) {
    return `Hisar emailed the invite to ${created.invite.email}.`;
  }
  if (created.emailError) {
    return `The email was not sent: ${created.emailError}. Copy the link and send it to ${created.invite.email} yourself.`;
  }
  return `Email is not set up on the server, so nobody was emailed. Copy the link and send it to ${created.invite.email} yourself.`;
}

export async function listInvites(): Promise<AdminInvite[]> {
  return (await request<{ invites: AdminInvite[] }>("GET", "/admin/invites")).invites;
}

export function createInvite(email: string, role: InviteRole): Promise<CreatedInvite> {
  return request("POST", "/admin/invites", { body: { email, role } });
}

export function revokeInvite(id: string): Promise<void> {
  return request("DELETE", `/admin/invites/${encodeURIComponent(id)}`);
}

export function resendInvite(id: string): Promise<CreatedInvite> {
  return request("POST", `/admin/invites/${encodeURIComponent(id)}/resend`);
}
