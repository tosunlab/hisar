import { getPreferenceValues } from "@raycast/api";
import { version } from "../../package.json";
import type { ConnectOptions } from "../hisar";

/** PrimaryAction is what ↵ does on an item with a password in Search Hisar. */
export type PrimaryAction = "copy-password" | "paste-password";

interface Preferences {
  socketPath?: string;
  primaryAction?: string;
}

export function getConnectOptions(): ConnectOptions {
  return { socketPath: getPreferenceValues<Preferences>().socketPath, clientVersion: version };
}

export function getPrimaryAction(): PrimaryAction {
  return getPreferenceValues<Preferences>().primaryAction === "paste-password" ? "paste-password" : "copy-password";
}
