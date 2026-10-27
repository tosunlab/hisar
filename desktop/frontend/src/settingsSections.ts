import { inject, type InjectionKey } from "vue";
import type { IconName } from "./components/AppIcon.vue";

export type SettingsSectionID = "general" | "security" | "ssh" | "developer" | "browser" | "people" | "organizations" | "devices" | "export" | "account" | "help";

export interface SettingsRow {
  id: string;
  title: string;
  keywords: string;
}

export interface SettingsSection {
  id: SettingsSectionID;
  label: string;
  icon: IconName;
  allRow: SettingsRow[];
}

export const allSettingsSection: SettingsSection[] = [
  {
    id: "general",
    label: "General",
    icon: "settings",
    allRow: [
      { id: "appearance", title: "Appearance", keywords: "theme light dark system mode" },
      { id: "theme-color", title: "Color", keywords: "theme colour accent tint neutral blue purple green" },
      { id: "website-icons", title: "Show website icons", keywords: "favicon logo item icon login privacy" },
      { id: "open-at-login", title: "Open Hisar at login", keywords: "startup login items menu bar launch" },
      { id: "version", title: "Check for updates", keywords: "version update release" },
      { id: "automatic-updates", title: "Automatically check for updates", keywords: "update daily" },
    ],
  },
  {
    id: "security",
    label: "Security",
    icon: "lock",
    allRow: [
      { id: "touch-id", title: "Unlock with Touch ID", keywords: "biometric fingerprint unlock" },
      { id: "auto-lock", title: "Auto-lock", keywords: "lock idle timeout minutes" },
      { id: "clipboard", title: "Clear copied values after 90 seconds", keywords: "clipboard copy clear" },
      { id: "change-master-password", title: "Change master password", keywords: "password change new rotate" },
      { id: "replace-secret-key", title: "Replace Secret Key", keywords: "secret key rotate new emergency kit leaked compromised" },
      { id: "emergency-kit", title: "Emergency Kit", keywords: "show copy scan qr secret key pdf recovery backup new device" },
    ],
  },
  {
    id: "ssh",
    label: "SSH & Git",
    icon: "key",
    allRow: [
      { id: "ssh-agent", title: "Hisar SSH agent", keywords: "ssh config use hisar for ssh identityagent socket keys" },
      { id: "ssh-remember", title: "Remember SSH approvals", keywords: "don't ask again hour until locks remember ssh approval duration" },
      { id: "compact-ssh-approvals", title: "Compact SSH approvals", keywords: "approval card notification dialog" },
      { id: "phone-approval", title: "Approve requests on my smart device", keywords: "phone iphone android smart device allow approvals approve requests" },
      { id: "commit-signing", title: "Remember commit signing", keywords: "git signature sign gpg always asks rebase remember commit tag" },
    ],
  },
  {
    id: "developer",
    label: "Developer tools",
    icon: "terminal",
    allRow: [
      { id: "command-line-tool", title: "hisar command-line tool", keywords: "cli install uninstall path terminal" },
      { id: "command-line-integration", title: "Let the command-line tool use Hisar", keywords: "cli terminal approve session" },
      { id: "command-line-launchers", title: "Launchers", keywords: "raycast launcher approved apps revoke" },
      { id: "coding-agents", title: "Coding agents", keywords: "hisar mcp claude code codex cursor omp tern plugin connect disconnect agent available to agents vaults" },
      { id: "agent-sign-in", title: "Website sign-in", keywords: "login website computer use hermes cua accessibility password chrome brave edge" },
      { id: "agent-grants", title: "Allowed without asking", keywords: "coding agent remembered approval grant revoke always hour day" },
      { id: "agent-sessions", title: "Running agents", keywords: "omp coding agent session revoke remembered secrets" },
      { id: "agent-hosts", title: "Agent hosts", keywords: "tern omp app forget coding agent command-line approval" },
    ],
  },
  {
    id: "browser",
    label: "Browser",
    icon: "globe",
    allRow: [
      { id: "browser-extension", title: "Browser extension", keywords: "chrome firefox brave edge arc vivaldi installed" },
      { id: "paired-browsers", title: "Paired browsers", keywords: "pair remove extension" },
      { id: "exact-host", title: "Only fill on the exact host", keywords: "autofill domain subdomain url matching" },
    ],
  },
  {
    id: "people",
    label: "People",
    icon: "users",
    allRow: [
      { id: "received-invitations", title: "Invitations for you", keywords: "accept decline share vault" },
      { id: "people", title: "People you share with", keywords: "security code fingerprint share" },
      { id: "sent-invitations", title: "Invitations you sent", keywords: "cancel pending share" },
      { id: "invite", title: "Invite to Hisar", keywords: "invite someone email invitation new account sign up" },
    ],
  },
  {
    id: "organizations",
    label: "Organizations",
    icon: "building",
    allRow: [
      { id: "organization-invitations", title: "Invitations to organizations", keywords: "accept decline join organization invite" },
      { id: "organizations", title: "Your organizations", keywords: "organization team teams company members admin vaults security code" },
      { id: "create-organization", title: "Create organization", keywords: "new organization team company" },
    ],
  },
  {
    id: "devices",
    label: "Devices",
    icon: "laptop",
    allRow: [{ id: "devices", title: "Signed-in devices", keywords: "revoke sign out mac iphone last seen" }],
  },
  {
    id: "export",
    label: "Import & Export",
    icon: "export",
    allRow: [
      { id: "import", title: "Import from another password manager", keywords: "1password bitwarden csv chrome firefox safari edge export" },
      { id: "export", title: "Export your data", keywords: "export backup json download data encrypted passphrase plain file" },
    ],
  },
  {
    id: "account",
    label: "Account",
    icon: "user",
    allRow: [
      { id: "account", title: "Account", keywords: "name email server sign out" },
      { id: "delete-account", title: "Delete account", keywords: "remove close erase gdpr data privacy" },
    ],
  },
  {
    id: "help",
    label: "Help",
    icon: "help",
    allRow: [
      { id: "support", title: "Contact support", keywords: "help email feedback bug problem question" },
      { id: "diagnostics", title: "Copy diagnostics", keywords: "help version system server device id debug report" },
    ],
  },
];

const selectedSectionStorageKey = "hisar.settings.selectedSection";

export function isRowMatching(section: SettingsSection, row: SettingsRow, query: string): boolean {
  const searchedText = `${section.label} ${row.title} ${row.keywords}`.toLowerCase();
  return query
    .toLowerCase()
    .split(/\s+/)
    .filter((word) => word !== "")
    .every((word) => searchedText.includes(word));
}

export function findAllMatchingRowID(query: string): Set<string> {
  const allMatchingRowID = new Set<string>();
  for (const section of allSettingsSection) {
    for (const row of section.allRow) {
      if (isRowMatching(section, row, query)) {
        allMatchingRowID.add(row.id);
      }
    }
  }
  return allMatchingRowID;
}

export function isSettingsSectionID(value: string | null): value is SettingsSectionID {
  return allSettingsSection.some((section) => section.id === value);
}

export function loadSelectedSectionID(): SettingsSectionID {
  try {
    const storedSectionID = window.localStorage.getItem(selectedSectionStorageKey);
    return isSettingsSectionID(storedSectionID) ? storedSectionID : "general";
  } catch {
    return "general";
  }
}

export function saveSelectedSectionID(sectionID: SettingsSectionID): void {
  try {
    window.localStorage.setItem(selectedSectionStorageKey, sectionID);
  } catch {
    // Storage can be unavailable; the section is then not remembered.
  }
}

export const settingsRowVisibilityKey: InjectionKey<(rowID: string) => boolean> = Symbol("settingsRowVisibility");

export function useSettingsRowVisibility(): (rowID: string) => boolean {
  return inject(settingsRowVisibilityKey, () => true);
}
