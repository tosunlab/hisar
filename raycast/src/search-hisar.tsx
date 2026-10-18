import { Action, ActionPanel, Icon, Keyboard, List } from "@raycast/api";
import { useState } from "react";
import type { Hisar, Item, Vault } from "./hisar";
import { copyField, pasteValue } from "./raycast/actions";
import { getCategory } from "./raycast/categories";
import { ErrorView } from "./raycast/feedback";
import { useHisar, useItemList } from "./raycast/hooks";
import { allVaults, groupItems, toWebsiteURL } from "./raycast/itemList";
import { getPrimaryAction, type PrimaryAction } from "./raycast/preferences";

export default function SearchHisar() {
  const hisar = useHisar();
  const { data: itemList, isLoading, error, revalidate } = useItemList(hisar);
  const [vaultFilter, setVaultFilter] = useState(allVaults);
  const primaryAction = getPrimaryAction();
  const vaultNameByID = new Map(itemList?.allVault.map((vault) => [vault.id, vault.name]));

  return (
    <List
      isLoading={isLoading}
      isShowingDetail={itemList !== undefined && itemList.allItem.length > 0}
      searchBarPlaceholder="Search Hisar"
      searchBarAccessory={<VaultDropdown allVault={itemList?.allVault ?? []} onChange={setVaultFilter} />}
    >
      {error ? (
        <ErrorView error={error} onRetry={revalidate} />
      ) : (
        itemList &&
        groupItems(itemList, vaultFilter).map((section) => (
          <List.Section key={section.id} title={section.title}>
            {section.allItem.map((item) => (
              <List.Item
                key={`${item.vaultID}/${item.itemID}`}
                title={item.title}
                icon={getCategory(item.category).icon}
                keywords={[item.username, ...item.allTag]}
                detail={<ItemDetail item={item} vaultName={vaultNameByID.get(item.vaultID) ?? ""} />}
                actions={<ItemActions hisar={hisar} item={item} primaryAction={primaryAction} onReload={revalidate} />}
              />
            ))}
          </List.Section>
        ))
      )}
    </List>
  );
}

function VaultDropdown({ allVault, onChange }: { allVault: Vault[]; onChange: (vaultFilter: string) => void }) {
  return (
    <List.Dropdown tooltip="Vault" onChange={onChange}>
      <List.Dropdown.Item title="All Vaults" value={allVaults} />
      {allVault.map((vault) => (
        <List.Dropdown.Item key={vault.id} title={vault.name} value={vault.id} />
      ))}
    </List.Dropdown>
  );
}

function ItemDetail({ item, vaultName }: { item: Item; vaultName: string }) {
  return (
    <List.Item.Detail
      metadata={
        <List.Item.Detail.Metadata>
          <List.Item.Detail.Metadata.Label title="Vault" text={vaultName} />
          <List.Item.Detail.Metadata.Label title="Category" text={getCategory(item.category).label} />
          {item.username !== "" && <List.Item.Detail.Metadata.Label title="Username" text={item.username} />}
          {item.allURL.map((url) => {
            const websiteURL = toWebsiteURL(url);
            return websiteURL ? (
              <List.Item.Detail.Metadata.Link key={url} title="Website" target={websiteURL} text={url} />
            ) : (
              <List.Item.Detail.Metadata.Label key={url} title="Website" text={url} />
            );
          })}
          {item.allTag.length > 0 && (
            <List.Item.Detail.Metadata.TagList title="Tags">
              {item.allTag.map((tag) => (
                <List.Item.Detail.Metadata.TagList.Item key={tag} text={tag} />
              ))}
            </List.Item.Detail.Metadata.TagList>
          )}
          {item.hasOneTimeCode && <List.Item.Detail.Metadata.Label title="One-time code" text="Yes" />}
        </List.Item.Detail.Metadata>
      }
    />
  );
}

interface ItemActionsProps {
  hisar: Hisar;
  item: Item;
  primaryAction: PrimaryAction;
  onReload: () => void;
}

function ItemActions({ hisar, item, primaryAction, onReload }: ItemActionsProps) {
  const copyPassword = (
    <Action
      key="copy-password"
      title="Copy Password"
      icon={Icon.Key}
      onAction={() => copyField(hisar, item, "password")}
    />
  );
  const pastePassword = (
    <Action
      key="paste-password"
      title="Paste Password"
      icon={Icon.Clipboard}
      onAction={() => pasteValue(() => hisar.readPassword(item))}
    />
  );
  const websiteURL = item.allURL.map(toWebsiteURL).find((url) => url !== undefined);

  return (
    <ActionPanel>
      {item.hasPassword && (
        <ActionPanel.Section>
          {primaryAction === "paste-password" ? [pastePassword, copyPassword] : [copyPassword, pastePassword]}
        </ActionPanel.Section>
      )}
      <ActionPanel.Section>
        {item.username !== "" && (
          <Action.CopyToClipboard
            title="Copy Username"
            content={item.username}
            shortcut={Keyboard.Shortcut.Common.Copy}
          />
        )}
        {item.hasOneTimeCode && (
          <Action
            title="Copy One-Time Code"
            icon={Icon.Clock}
            shortcut={{ modifiers: ["cmd"], key: "t" }}
            onAction={() => copyField(hisar, item, "one-time code")}
          />
        )}
        {item.hasOneTimeCode && (
          <Action
            title="Paste One-Time Code"
            icon={Icon.Clock}
            shortcut={{ modifiers: ["cmd", "shift"], key: "t" }}
            onAction={() => pasteValue(async () => (await hisar.getCode(item)).code)}
          />
        )}
        {websiteURL && (
          <Action.OpenInBrowser title="Open Website" url={websiteURL} shortcut={Keyboard.Shortcut.Common.Open} />
        )}
        {item.allReference.length > 0 && (
          <ActionPanel.Submenu
            title="Copy Reference"
            icon={Icon.Link}
            shortcut={{ modifiers: ["cmd", "shift"], key: "r" }}
          >
            {item.allReference.map((reference) => (
              <Action.CopyToClipboard key={reference.field} title={reference.field} content={reference.reference} />
            ))}
          </ActionPanel.Submenu>
        )}
      </ActionPanel.Section>
      <ActionPanel.Section>
        <Action
          title="Reload Items"
          icon={Icon.ArrowClockwise}
          shortcut={Keyboard.Shortcut.Common.Refresh}
          onAction={onReload}
        />
      </ActionPanel.Section>
    </ActionPanel>
  );
}
