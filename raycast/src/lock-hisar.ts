import { showHUD } from "@raycast/api";
import { toHisarError, withHisar } from "./hisar";
import { showHisarError } from "./raycast/feedback";
import { getConnectOptions } from "./raycast/preferences";

/** LockHisar locks a running Hisar; it never starts Hisar just to lock it. */
export default async function LockHisar(): Promise<void> {
  try {
    await withHisar(getConnectOptions(), (hisar) => hisar.lock());
    await showHUD("Hisar is locked");
  } catch (error) {
    if (toHisarError(error).code === "app-not-running") await showHUD("Hisar is not running");
    else await showHisarError(error);
  }
}
