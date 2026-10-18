import { getPreferenceValues } from "@raycast/api";
import { version } from "../../package.json";
import type { ConnectOptions } from "../hisar";
import { parseItemAction, type ItemActionPreferences } from "./itemActions";

interface Preferences {
  socketPath?: string;
  primaryAction?: string;
  secondaryAction?: string;
  closeWindowAfterCopying?: boolean;
}

export function getConnectOptions(): ConnectOptions {
  return { socketPath: getPreferenceValues<Preferences>().socketPath, clientVersion: version };
}

export function getItemActionPreferences(): ItemActionPreferences {
  const { primaryAction, secondaryAction } = getPreferenceValues<Preferences>();
  return {
    primary: parseItemAction(primaryAction, "copy-password"),
    secondary: parseItemAction(secondaryAction, "paste-password"),
  };
}

/** isClosingWindowAfterCopy tells whether a copy closes Raycast with a HUD, or keeps it open with a toast. */
export function isClosingWindowAfterCopy(): boolean {
  return getPreferenceValues<Preferences>().closeWindowAfterCopying ?? true;
}
