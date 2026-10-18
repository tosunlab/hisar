import { showToast, Toast } from "@raycast/api";
import { usePromise } from "@raycast/utils";
import { useEffect, useState } from "react";
import { Hisar, type ItemReference } from "../hisar";
import { showHisarError } from "./feedback";
import { getConnectOptions } from "./preferences";

/** useHisar keeps one connection to Hisar for the command and closes it when the command closes. */
export function useHisar(): Hisar {
  const [hisar] = useState(() => new Hisar(getConnectOptions()));
  useEffect(() => () => hisar.close(), [hisar]);
  return hisar;
}

/**
 * useItemList loads every item's metadata into memory only (usePromise: Raycast's cache would write it to disk). While
 * Hisar asks whether Raycast may use it, a toast says so; once answered, Hisar opens Raycast again to show the list.
 */
export function useItemList(hisar: Hisar) {
  return usePromise(
    async () => {
      const approvalToast = (await hisar.isApproved())
        ? undefined
        : await showToast({
            style: Toast.Style.Animated,
            title: "Approve Raycast in Hisar",
            message: "With Touch ID, your master password or your iPhone",
          });
      try {
        return await hisar.listItems();
      } finally {
        await approvalToast?.hide();
      }
    },
    [],
    { onError: showHisarError },
  );
}

export interface LiveOneTimeCode {
  /** code is undefined while it loads and once it expired. */
  code?: string;
  secondsLeft: number;
}

/** useOneTimeCode keeps an item's current code in memory, counts down to now and fetches the next one when it expires. */
export function useOneTimeCode(hisar: Hisar, item: ItemReference, now: number): LiveOneTimeCode {
  const { data, isLoading, revalidate } = usePromise(
    (vaultID: string, itemID: string) => hisar.getCode({ vaultID, itemID }),
    [item.vaultID, item.itemID],
    { onError: showHisarError },
  );
  // Rounded up, so a new 30-second code starts at 30 and the countdown never shows 0.
  const secondsLeft = data ? Math.max(0, Math.ceil((data.expiresAt.getTime() - now) / 1000)) : 0;
  const isExpired = data !== undefined && secondsLeft === 0;
  // usePromise drops the data when a fetch fails, so a failing fetch is not retried every second.
  useEffect(() => {
    if (isExpired && !isLoading) void revalidate();
  }, [isExpired, isLoading, revalidate]);
  return { code: isExpired ? undefined : data?.code, secondsLeft };
}
