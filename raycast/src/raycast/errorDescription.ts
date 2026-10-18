import type { ErrorCode, HisarError } from "../hisar";

/** HisarAction is what a failure offers; both open Hisar, whose window offers its own update. */
export type HisarAction = "open-hisar" | "update-hisar";

export interface ErrorDescription {
  title: string;
  message: string;
  action?: HisarAction;
}

interface CodeDescription {
  title: string;
  /** message replaces the error's own when that one is written for the command-line tool. */
  message?: string;
  action?: HisarAction;
}

const turnOnIntegration = "In Hisar, open Settings → Developer tools and turn on Let the command-line tool use Hisar.";

const descriptionByCode: Record<ErrorCode, CodeDescription> = {
  "invalid-request": { title: "Hisar refused the request" },
  "integration-disabled": {
    title: "Hisar does not let Raycast in",
    message: turnOnIntegration,
    action: "open-hisar",
  },
  "signed-out": { title: "Sign in to Hisar", action: "open-hisar" },
  locked: { title: "Hisar is locked", action: "open-hisar" },
  denied: { title: "Hisar denied the request" },
  offline: { title: "Hisar is offline", action: "open-hisar" },
  "not-found": { title: "Not found in Hisar" },
  internal: { title: "Hisar could not answer" },
  "update-required": { title: "Hisar needs an update", action: "update-hisar" },
  "app-not-installed": { title: "Hisar is not installed" },
  "app-not-running": { title: "Hisar is not running", action: "open-hisar" },
  "connection-lost": { title: "Lost the connection to Hisar", action: "open-hisar" },
  timeout: { title: "Hisar did not answer in time", action: "open-hisar" },
};

/** describeHisarError says what failed in a toast or an empty list, and which action helps. */
export function describeHisarError(error: HisarError): ErrorDescription {
  const description = descriptionByCode[error.code];
  return { title: description.title, message: description.message ?? error.message, action: description.action };
}
