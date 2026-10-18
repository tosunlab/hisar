import { Action, ActionPanel, closeMainWindow, Color, Icon, Keyboard, List, open } from "@raycast/api";
import { useCachedState } from "@raycast/utils";
import type { Hisar, Item, ItemList } from "../hisar";
import { copyField, openItem, pasteValue } from "./actions";
import { getCategory } from "./categories";
import { ErrorView } from "./feedback";
import { useItemList } from "./hooks";
import { listShareableFields, orderItemActions, type ItemAction, type ItemActionPreferences } from "./itemActions";
import {
  allItemsFilterValue,
  findItemFilter,
  findWebsiteURL,
  getItemKeywords,
  groupItems,
  listCategories,
  toCategoryFilterValue,
  toVaultFilterValue,
  toWebsiteURL,
} from "./itemList";
import { getItemActionPreferences } from "./preferences";
import { ShareItemForm } from "./shareItemForm";

export interface ItemListViewProps {
  hisar: Hisar;
  /** filterValue is the dropdown's selection: every item, a vault or a category. */
  filterValue: string;
  onFilterChange: (filterValue: string) => void;
  navigationTitle?: string;
}

/** ItemListView lists every item Hisar lets Raycast see, in memory only, with the actions the preferences order. */
export function ItemListView({ hisar, filterValue, onFilterChange, navigationTitle }: ItemListViewProps) {
  const { data: itemList, isLoading, error, revalidate } = useItemList(hisar);
  const [isShowingDetail, setShowingDetail] = useCachedState("isShowingDetail", false);
  const preferences = getItemActionPreferences();
  const vaultNameByID = new Map(itemList?.allVault.map((vault) => [vault.id, vault.name]));
  const filter = itemList && findItemFilter(itemList, filterValue);

  return (
    <List
      isLoading={isLoading}
      navigationTitle={navigationTitle}
      isShowingDetail={isShowingDetail && itemList !== undefined && itemList.allItem.length > 0}
      searchBarPlaceholder="Search by title, username, vault or website"
      searchBarAccessory={
        itemList && filter && <ItemFilterDropdown itemList={itemList} value={filter.value} onChange={onFilterChange} />
      }
    >
      {error ? (
        <ErrorView error={error} onRetry={revalidate} />
      ) : (
        <>
          <List.EmptyView
            icon={Icon.MagnifyingGlass}
            title="No items found"
            description="Items you add in Hisar show up here."
          />
          {itemList &&
            filter &&
            groupItems(itemList, filter).map((section) => (
              <List.Section key={section.id} title={section.title}>
                {section.allItem.map((item) => {
                  const vaultName = vaultNameByID.get(item.vaultID) ?? "";
                  return (
                    <List.Item
                      key={`${item.vaultID}/${item.itemID}`}
                      title={item.title}
                      subtitle={item.username}
                      icon={getCategory(item.category).icon}
                      keywords={getItemKeywords(item, vaultName)}
                      accessories={[
                        ...(item.isFavorite ? [{ icon: { source: Icon.Star, tintColor: Color.Yellow } }] : []),
                        { text: vaultName },
                      ]}
                      detail={<ItemDetail item={item} vaultName={vaultName} />}
                      actions={
                        <ItemActionPanel
                          hisar={hisar}
                          item={item}
                          preferences={preferences}
                          isShowingDetail={isShowingDetail}
                          onToggleDetail={() => setShowingDetail(!isShowingDetail)}
                          onReload={revalidate}
                        />
                      }
                    />
                  );
                })}
              </List.Section>
            ))}
        </>
      )}
    </List>
  );
}

interface ItemFilterDropdownProps {
  itemList: ItemList;
  value: string;
  onChange: (filterValue: string) => void;
}

function ItemFilterDropdown({ itemList, value, onChange }: ItemFilterDropdownProps) {
  return (
    <List.Dropdown tooltip="Vault or Category" value={value} onChange={onChange}>
      <List.Dropdown.Item title="All Items" icon={Icon.AppWindowGrid3x3} value={allItemsFilterValue} />
      <List.Dropdown.Section title="Vaults">
        {itemList.allVault.map((vault) => (
          <List.Dropdown.Item key={vault.id} title={vault.name} icon={Icon.Lock} value={toVaultFilterValue(vault.id)} />
        ))}
      </List.Dropdown.Section>
      <List.Dropdown.Section title="Categories">
        {listCategories(itemList).map((category) => {
          const { label, icon } = getCategory(category);
          return (
            <List.Dropdown.Item key={category} title={label} icon={icon} value={toCategoryFilterValue(category)} />
          );
        })}
      </List.Dropdown.Section>
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

interface ItemActionView {
  title: string;
  icon: Icon;
  shortcut: Keyboard.Shortcut;
  run: (hisar: Hisar, item: Item) => Promise<void>;
}

/** Each action keeps its shortcut wherever the preferences put it; the first two also answer Return and ⌘ Return. */
const viewByItemAction: Record<ItemAction, ItemActionView> = {
  "open-in-hisar": {
    title: "Open in Hisar",
    icon: Icon.AppWindow,
    shortcut: { modifiers: ["cmd", "shift"], key: "o" },
    run: (hisar, item) => openItem(hisar, item, false),
  },
  "open-in-browser": {
    title: "Open in Browser",
    icon: Icon.Globe,
    shortcut: { modifiers: ["opt"], key: "return" },
    run: async (_, item) => {
      const websiteURL = findWebsiteURL(item);
      if (websiteURL === undefined) return;
      await open(websiteURL);
      await closeMainWindow();
    },
  },
  "copy-username": {
    title: "Copy Username",
    icon: Icon.Person,
    shortcut: { modifiers: ["cmd", "shift"], key: "c" },
    run: (hisar, item) => copyField(hisar, item, "username"),
  },
  "copy-password": {
    title: "Copy Password",
    icon: Icon.Key,
    shortcut: { modifiers: ["cmd", "opt"], key: "c" },
    run: (hisar, item) => copyField(hisar, item, "password"),
  },
  "copy-one-time-code": {
    title: "Copy One-Time Code",
    icon: Icon.Clock,
    shortcut: { modifiers: ["cmd", "ctrl"], key: "c" },
    run: (hisar, item) => copyField(hisar, item, "one-time code"),
  },
  "paste-username": {
    title: "Paste Username",
    icon: Icon.Person,
    shortcut: { modifiers: ["cmd", "shift"], key: "v" },
    run: (_, item) => pasteValue(async () => item.username),
  },
  "paste-password": {
    title: "Paste Password",
    icon: Icon.Key,
    shortcut: { modifiers: ["cmd", "opt"], key: "v" },
    run: (hisar, item) => pasteValue(() => hisar.readPassword(item)),
  },
  "paste-one-time-code": {
    title: "Paste One-Time Code",
    icon: Icon.Clock,
    shortcut: { modifiers: ["cmd", "ctrl"], key: "v" },
    run: (hisar, item) => pasteValue(async () => (await hisar.getCode(item)).code),
  },
};

interface ItemActionPanelProps {
  hisar: Hisar;
  item: Item;
  preferences: ItemActionPreferences;
  isShowingDetail: boolean;
  onToggleDetail: () => void;
  onReload: () => void;
}

function ItemActionPanel({
  hisar,
  item,
  preferences,
  isShowingDetail,
  onToggleDetail,
  onReload,
}: ItemActionPanelProps) {
  return (
    <ActionPanel>
      <ActionPanel.Section>
        {orderItemActions(item, preferences).map((action) => {
          const { title, icon, shortcut, run } = viewByItemAction[action];
          return (
            <Action key={action} title={title} icon={icon} shortcut={shortcut} onAction={() => run(hisar, item)} />
          );
        })}
      </ActionPanel.Section>
      <ActionPanel.Section>
        <Action
          title="Edit in Hisar"
          icon={Icon.Pencil}
          shortcut={Keyboard.Shortcut.Common.Edit}
          onAction={() => openItem(hisar, item, true)}
        />
        {listShareableFields(item).length > 0 && (
          <Action.Push
            title="Share Item…"
            icon={Icon.Link}
            // ⌘⇧S is Share Item in 1Password's extension; this action duplicates nothing.
            // eslint-disable-next-line @raycast/prefer-common-shortcut
            shortcut={{ modifiers: ["cmd", "shift"], key: "s" }}
            target={<ShareItemForm hisar={hisar} item={item} />}
          />
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
          title={isShowingDetail ? "Hide Details" : "Show Details"}
          icon={Icon.Sidebar}
          shortcut={Keyboard.Shortcut.Common.ToggleQuickLook}
          onAction={onToggleDetail}
        />
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
