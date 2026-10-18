import { Action, ActionPanel, Icon, List } from "@raycast/api";
import { useEffect, useState } from "react";
import type { Hisar, Item } from "./hisar";
import { copyField, pasteValue } from "./raycast/actions";
import { getCategory } from "./raycast/categories";
import { ErrorView } from "./raycast/feedback";
import { useHisar, useItemList, useOneTimeCode } from "./raycast/hooks";

export default function OneTimeCodes() {
  const hisar = useHisar();
  const { data: itemList, isLoading, error, revalidate } = useItemList(hisar);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  const allItem = (itemList?.allItem ?? [])
    .filter((item) => item.hasOneTimeCode)
    .toSorted((left, right) => left.title.localeCompare(right.title));

  return (
    <List isLoading={isLoading} searchBarPlaceholder="Search one-time codes">
      {error ? (
        <ErrorView error={error} onRetry={revalidate} />
      ) : (
        <>
          <List.EmptyView
            icon={Icon.Clock}
            title="No items found"
            description="Items with a one-time code in Hisar show up here."
          />
          {allItem.map((item) => (
            <OneTimeCodeItem key={`${item.vaultID}/${item.itemID}`} hisar={hisar} item={item} now={now} />
          ))}
        </>
      )}
    </List>
  );
}

function OneTimeCodeItem({ hisar, item, now }: { hisar: Hisar; item: Item; now: number }) {
  const { code, secondsLeft } = useOneTimeCode(hisar, item, now);
  return (
    <List.Item
      title={item.title}
      subtitle={item.username}
      icon={getCategory(item.category).icon}
      keywords={item.allTag}
      accessories={
        code
          ? [{ text: code.replace(/^(\d{3})(\d{3})$/, "$1 $2") }, { text: `${secondsLeft} s`, icon: Icon.Clock }]
          : [{ text: "…" }]
      }
      actions={
        <ActionPanel>
          <Action
            title="Copy One-Time Code"
            icon={Icon.CopyClipboard}
            onAction={() => copyField(hisar, item, "one-time code")}
          />
          <Action
            title="Paste One-Time Code"
            icon={Icon.Clipboard}
            onAction={() => pasteValue(async () => (await hisar.getCode(item)).code)}
          />
        </ActionPanel>
      }
    />
  );
}
