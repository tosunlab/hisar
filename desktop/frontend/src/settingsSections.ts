import { inject, type InjectionKey } from "vue";
import type { IconName } from "./components/AppIcon.vue";

export type SettingsSectionID = "general" | "security" | "ssh" | "developer" | "browser" | "people" | "devices" | "account";

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
      { id: "open-at-login", title: "Open Hisar at login", keywords: "startup login items menu bar launch" },
      { id: "version", title: "Check for updates", keywords: "version update release" },
      { id: "automatic-updates", title: "Automatically check for updates", keywords: "update daily" },
      { id: "import", title: "Import from another password manager", keywords: "1password bitwarden csv chrome firefox safari edge export" },
    ],
  },
  {
    id: "security",
    label: "Security",
    icon: "lock",
    allRow: [
      { id: "touch-id", title: "Unlock with Touch ID", keywords: "biometric fingerprint unlock" },
      { id: "auto-lock", title: "Auto-lock", keywords: "lock idle timeout clipboard minutes" },
      { id: "emergency-kit", title: "Emergency Kit", keywords: "secret key pdf recovery backup new device" },
    ],
  },
  {
    id: "ssh",
    label: "SSH & Git",
    icon: "key",
    allRow: [
      { id: "ssh-agent", title: "Hisar SSH agent", keywords: "ssh config use hisar for ssh identityagent socket keys" },
      { id: "compact-ssh-approvals", title: "Compact SSH approvals", keywords: "approval card notification dialog" },
      { id: "phone-approval", title: "Allow approvals from my iPhone", keywords: "phone iphone approve requests" },
      { id: "commit-signing", title: "Commit and tag signing", keywords: "git signature sign gpg always asks" },
    ],
  },
  {
    id: "developer",
    label: "Developer tools",
    icon: "terminal",
    allRow: [
      { id: "command-line-tool", title: "hisar command-line tool", keywords: "cli install uninstall path terminal" },
      { id: "command-line-integration", title: "Let the command-line tool use Hisar", keywords: "cli terminal approve session" },
      { id: "coding-agents", title: "Coding agents", keywords: "hisar mcp claude code cursor agent available to agents vaults" },
    ],
  },
  {
    id: "browser",
    label: "Browser",
    icon: "globe",
    allRow: [
      { id: "browser-extension", title: "Browser extension", keywords: "chrome brave edge arc vivaldi installed" },
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
      { id: "invite", title: "Invite someone to Hisar", keywords: "email invitation account" },
    ],
  },
  {
    id: "devices",
    label: "Devices",
    icon: "laptop",
    allRow: [{ id: "devices", title: "Signed-in devices", keywords: "revoke sign out mac iphone last seen" }],
  },
  {
    id: "account",
    label: "Account",
    icon: "user",
    allRow: [{ id: "account", title: "Account", keywords: "name email server sign out" }],
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
