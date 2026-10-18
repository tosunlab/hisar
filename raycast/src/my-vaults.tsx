import { Action, ActionPanel, Icon, Keyboard, List } from "@raycast/api";
import { useState } from "react";
import type { Hisar, Vault } from "./hisar";
import { ErrorView } from "./raycast/feedback";
import { useHisar, useItemList } from "./raycast/hooks";
import { toVaultFilterValue } from "./raycast/itemList";
import { ItemListView } from "./raycast/itemListView";

export default function MyVaults() {
  const hisar = useHisar();
  const { data: itemList, isLoading, error, revalidate } = useItemList(hisar);

  return (
    <List isLoading={isLoading} searchBarPlaceholder="Search vaults">
      {error ? (
        <ErrorView error={error} onRetry={revalidate} />
      ) : (
        <>
          <List.EmptyView icon={Icon.Lock} title="No vaults found" />
          {itemList?.allVault.map((vault) => {
            const count = itemList.allItem.filter((item) => item.vaultID === vault.id).length;
            return (
              <List.Item
                key={vault.id}
                title={vault.name}
                icon={Icon.Lock}
                accessories={[{ text: count === 1 ? "1 item" : `${count} items` }]}
                actions={
                  <ActionPanel>
                    <Action.Push
                      title="Show Items"
                      icon={Icon.List}
                      target={<VaultItems hisar={hisar} vault={vault} />}
                    />
                    <Action
                      title="Reload Vaults"
                      icon={Icon.ArrowClockwise}
                      shortcut={Keyboard.Shortcut.Common.Refresh}
                      onAction={revalidate}
                    />
                  </ActionPanel>
                }
              />
            );
          })}
        </>
      )}
    </List>
  );
}

function VaultItems({ hisar, vault }: { hisar: Hisar; vault: Vault }) {
  const [filterValue, setFilterValue] = useState(toVaultFilterValue(vault.id));
  return (
    <ItemListView
      hisar={hisar}
      filterValue={filterValue}
      onFilterChange={setFilterValue}
      navigationTitle={vault.name}
    />
  );
}
