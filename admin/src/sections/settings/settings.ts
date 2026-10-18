import { request } from "../../api";

// AdminSetting is one instance setting as GET /api/v1/admin/settings returns it; a secret's value is {set}.
export interface AdminSetting {
  value: unknown;
  source: "default" | "database" | "environment";
  environmentVariable?: string;
}

export type AdminSettings = Record<string, AdminSetting>;

export interface SettingField {
  name: string;
  label: string;
  kind: "text" | "number" | "password" | "invitePolicy";
  placeholder?: string;
  hint?: string;
}

export interface SettingGroup {
  id: string;
  title: string;
  description: string;
  fields: readonly SettingField[];
}

// FormValues holds what the inputs show; number inputs give numbers, the others text.
export type FormValues = Record<string, string | number>;

export const settingGroups: readonly SettingGroup[] = [
  {
    id: "email",
    title: "Email",
    description: "The SMTP server that sends invites and sharing notifications. Leave the host empty to turn email off.",
    fields: [
      { name: "smtp.host", label: "SMTP host", kind: "text", placeholder: "smtp.example.com" },
      { name: "smtp.port", label: "Port", kind: "number" },
      { name: "smtp.username", label: "Username", kind: "text" },
      { name: "smtp.password", label: "Password", kind: "password" },
      { name: "smtp.from", label: "Sender", kind: "text", placeholder: "Hisar <noreply@example.com>" },
    ],
  },
  {
    id: "sign-up",
    title: "Sign-up",
    description: "Who can invite people to the Hisar server from the apps. Sharing with someone who has no account waits until they get one.",
    fields: [{ name: "invitePolicy", label: "Who can invite people", kind: "invitePolicy" }],
  },
  {
    id: "app-versions",
    title: "App versions",
    description: "Older apps must update before they can reach the server. Leave a field empty for no minimum.",
    fields: [
      { name: "minimumVersions.desktopMacOS", label: "Mac app", kind: "text", placeholder: "No minimum" },
      { name: "minimumVersions.iOS", label: "iPhone app", kind: "text", placeholder: "No minimum" },
      { name: "minimumVersions.cli", label: "Command-line tool", kind: "text", placeholder: "No minimum" },
      { name: "minimumVersions.extension", label: "Browser extension", kind: "text", placeholder: "No minimum" },
    ],
  },
  {
    id: "security",
    title: "Security",
    description: "How long sign-ins last, how fast one address may try to sign in, and how long the server keeps old records.",
    fields: [
      { name: "sessionLifetimeDays", label: "Session lifetime", kind: "number", hint: "Days a device stays signed in without using Hisar." },
      { name: "authBurst", label: "Auth burst", kind: "number", hint: "Sign-in attempts one IP address may make at once, then one every 6 seconds." },
      { name: "retention.historyDays", label: "Keep item history", kind: "number", hint: "Days" },
      { name: "retention.auditDays", label: "Keep the audit log", kind: "number", hint: "Days, at least 30" },
      { name: "retention.expiredInvitesDays", label: "Keep expired invitations", kind: "number", hint: "Days" },
    ],
  },
];

// isSecretSet reports whether a secret setting, which the server answers as {set}, holds a value.
export function isSecretSet(setting: AdminSetting): boolean {
  const value = setting.value;
  return typeof value === "object" && value !== null && "set" in value && value.set === true;
}

export const invitePolicyLabels: Readonly<Record<string, string>> = { anyone: "Anyone with an account", admins: "Admins only" };

// toFormValue is what an input shows for a setting; a password input starts empty and keeps the saved password.
export function toFormValue(field: SettingField, setting: AdminSetting): string | number {
  if (field.kind === "password") {
    return "";
  }
  return field.kind === "number" ? Number(setting.value) : String(setting.value ?? "");
}

// listChanges returns the group's fields whose input differs from the saved value; fields the environment sets never change.
export function listChanges(group: SettingGroup, settings: AdminSettings, form: FormValues): Record<string, unknown> {
  const changes: Record<string, unknown> = {};
  for (const field of group.fields) {
    const setting = settings[field.name];
    if (!setting || setting.source === "environment") {
      continue;
    }
    const text = String(form[field.name] ?? "");
    if (field.kind === "password") {
      if (text !== "") {
        changes[field.name] = text;
      }
      continue;
    }
    if (text.trim() === String(toFormValue(field, setting))) {
      continue;
    }
    const number = Number(text);
    changes[field.name] = field.kind === "number" && text.trim() !== "" && Number.isInteger(number) ? number : text.trim();
  }
  return changes;
}

// isEmailFromEnvironment reports whether HISAR_SMTP_* variables set Email; the server then refuses every Email field.
export function isEmailFromEnvironment(settings: AdminSettings): boolean {
  return settingGroups[0].fields.some((field) => settings[field.name]?.source === "environment");
}

// needsPasswordAgain reports whether saving Email as typed would clear the saved SMTP password: the server clears it
// when the host, port or username changes and the same save does not set it.
export function needsPasswordAgain(settings: AdminSettings, form: FormValues): boolean {
  const changes = listChanges(settingGroups[0], settings, form);
  return (
    isSecretSet(settings["smtp.password"]) && !("smtp.password" in changes) && ["smtp.host", "smtp.port", "smtp.username"].some((name) => name in changes)
  );
}

export async function getSettings(): Promise<AdminSettings> {
  return (await request<{ settings: AdminSettings }>("GET", "/admin/settings")).settings;
}

export async function saveSettings(changes: Record<string, unknown>): Promise<AdminSettings> {
  return (await request<{ settings: AdminSettings }>("PUT", "/admin/settings", { body: { settings: changes } })).settings;
}

export function sendTestEmail(to: string): Promise<void> {
  return request("POST", "/admin/settings/test-email", { body: { to } });
}
