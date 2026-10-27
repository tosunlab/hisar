<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  clearError,
  copyItemField,
  countPendingInvitations,
  createOrganizationVault,
  createVault,
  duplicateItem,
  hasShareableField,
  filterAllFavorite,
  filterAllItem,
  listAllTag,
  loadAllOrganizationQuietly,
  lock,
  lockOnShortcut,
  openImport,
  openItemShareLinkDialog,
  openShareLinks,
  openWebsite,
  openSettings,
  openSharing,
  selectFavoriteItem,
  selectFavorites,
  selectItem,
  selectTag,
  selectVault,
  setItemFavorite,
  startCreatingItem,
  startCreatingCard,
  startCreatingIdentity,
  startCreatingAPICredential,
  startCreatingSoftwareLicense,
  startCreatingServer,
  startCreatingDatabase,
  startCreatingAddress,
  startCreatingSecureNote,
  startCreatingSSHKey,
  startEditingItem,
  store,
  syncInBackground,
  syncInterval,
  takeErrorMessage,
  toggleSelectedItemFavorite,
} from "../store";
import { api, type FavoriteItem, type ItemDetail, type VaultSummary } from "../api";
import {
  addressCategory,
  apiCredentialCategory,
  apiCredentialFieldName,
  cardFieldName,
  creditCardCategory,
  databaseCategory,
  formatAddressSummary,
  formatAPICredentialSummary,
  formatCardSummary,
  formatDatabaseSummary,
  formatIdentitySummary,
  formatKeyType,
  formatServerSummary,
  formatSoftwareLicenseSummary,
  getDomain,
  getInitials,
  getItemIcon,
  getWebsiteIconURL,
  identityCategory,
  isMacPlatform,
  secureNoteCategory,
  serverCategory,
  softwareLicenseCategory,
  softwareLicenseFieldName,
  sshKeyCategory,
} from "../util";
import { allColumnBound, detailMinWidth, loadColumnWidths, saveColumnWidths } from "../columnWidths";
import AppIcon from "./AppIcon.vue";
import ColumnResizer from "./ColumnResizer.vue";
import HisarLogo from "./HisarLogo.vue";
import ItemDetailPanel from "./ItemDetailPanel.vue";
import ItemEditor from "./ItemEditor.vue";
import ConfirmDeleteDialog from "./ConfirmDeleteDialog.vue";
import DiscardChangesDialog from "./DiscardChangesDialog.vue";
import ContextMenu, { type ContextMenuAction } from "./ContextMenu.vue";
import MoveItemDialog from "./MoveItemDialog.vue";
import ImportPanel from "./ImportPanel.vue";
import ItemTile from "./ItemTile.vue";
import SettingsPanel from "./SettingsPanel.vue";
import OrganizationAlert from "./OrganizationAlert.vue";
import ReadIssuesPanel from "./ReadIssuesPanel.vue";
import OrganizationBanner from "./OrganizationBanner.vue";
import RemovedVaultBanner from "./RemovedVaultBanner.vue";
import SharedLinksPanel from "./SharedLinksPanel.vue";
import ShareLinkDialog from "./ShareLinkDialog.vue";
import SSHKeyForm from "./SSHKeyForm.vue";
import VaultDialog from "./VaultDialog.vue";
import VaultMenu from "./VaultMenu.vue";
import VaultSharingPanel from "./VaultSharingPanel.vue";

const personalVaultNotice = "Personal vaults are private. Create a shared vault to share items with other people.";

const columnWidths = ref(loadColumnWidths());
const layoutStyle = computed(() => ({
  gridTemplateColumns: `minmax(${allColumnBound.sidebar.min}px, ${columnWidths.value.sidebar}px) 0 minmax(${allColumnBound.list.min}px, ${columnWidths.value.list}px) 0 minmax(${detailMinWidth}px, 1fr)`,
}));

interface TeamChoice {
  key: string;
  organizationID: string;
  teamID: string;
  teamName: string;
  label: string;
}

interface VaultGroup {
  organizationID: string;
  name: string;
  allVault: VaultSummary[];
}

const newVaultName = ref("");
// newVaultWhere is "" for the user's own vault, or the key of the team that gets the new organization vault.
const newVaultWhere = ref("");
// isLoadingTeams is true while the new-vault form loads the teams for Where; newVaultTeamLoad counts the loads so only the
// latest one ends it.
const isLoadingTeams = ref(false);
let newVaultTeamLoad = 0;
// newVaultErrorMessage is why the last Create failed, shown in the form.
const newVaultErrorMessage = ref("");
const searchInput = ref<HTMLInputElement | null>(null);
const isNewItemMenuOpen = ref(false);
const newItemMenu = ref<HTMLElement | null>(null);
const shortcutModifier = isMacPlatform() ? "⌘" : "Ctrl+";
const shiftModifier = isMacPlatform() ? "⇧" : "Shift+";
const itemList = ref<HTMLElement | null>(null);

const allVisibleItem = computed(() => filterAllItem(store.allItem.filter((item) => !item.readIssue), store.searchQuery, store.selectedTag));
const unreadableItemCount = computed(() => store.allItem.filter((item) => item.readIssue).length);
const healthyItemCount = computed(() => store.allItem.length - unreadableItemCount.value);
const allVisibleFavorite = computed(() => filterAllFavorite(store.allFavorite, store.searchQuery));
const listEntryIndex = computed(() => Math.max(0, store.isFavoritesSelected
  ? allVisibleFavorite.value.findIndex(isSelectedFavorite)
  : allVisibleItem.value.findIndex((item) => item.id === store.selectedItem?.id)));
const allTag = computed(() => listAllTag(store.allItem));
// inbox gathers what does not need the sidebar's room: invitations to answer, shares waiting on someone's Hisar, and
// organization vault keys on their way. actionCount is what waits for the user, as on Settings' badges; the row opens
// the Settings section where that is answered first.
const inbox = computed(() => {
  const allAcceptedSent = store.shareInvitations.sent.filter((invitation) => invitation.state === "accepted");
  const shareActionCount = countPendingInvitations("people");
  const shareCount = store.shareInvitations.received.length + allAcceptedSent.length + (store.shareInvitations.waiting?.length ?? 0);
  const organizationActionCount = countPendingInvitations("organizations");
  const waitingKeyCount = store.waitingVaultKeys.waiting.filter((waiting) => !store.allVault.some((vault) => vault.id === waiting.vaultId)).length;
  const isPeopleFirst = shareActionCount > 0 || (organizationActionCount === 0 && shareCount > 0);
  return {
    entryCount: shareCount + organizationActionCount + waitingKeyCount,
    actionCount: shareActionCount + organizationActionCount,
    sectionID: isPeopleFirst ? "people" : "organizations",
  };
});
const emptyListMessage = computed(() => {
  if (store.searchQuery) {
    return "No items match your search.";
  }
  return store.allVault.some((vault) => vault.readIssues?.length) ? "No readable matches. Some records could not be opened." : store.selectedTag ? `No items are tagged ${store.selectedTag}.` : "This vault is empty.";
});
const selectedVault = computed(() => store.allVault.find((vault) => vault.id === store.selectedVaultID));
// canShareVault: shared and organization vaults have a sharing panel, personal vaults the Private badge.
const canShareVault = computed(() => selectedVault.value !== undefined && selectedVault.value.type !== "personal");
const canEdit = computed(() => selectedVault.value !== undefined && selectedVault.value.role !== "reader");
const canCreate = computed(() => canEdit.value && !store.isFavoritesSelected);
// allVaultGroup is the sidebar's vaults: the user's own and shared vaults, then a group per organization by name.
const allVaultGroup = computed(() => {
  const groupByID = new Map<string, VaultGroup>();
  for (const vault of store.allVault) {
    if (!vault.organizationId) {
      continue;
    }
    const name = store.organizations.organizations.find((organization) => organization.id === vault.organizationId)?.name ?? "Organization";
    const group = groupByID.get(vault.organizationId) ?? { organizationID: vault.organizationId, name, allVault: [] };
    group.allVault.push(vault);
    groupByID.set(vault.organizationId, group);
  }
  const allOrganizationGroup = [...groupByID.values()].sort((first, second) => first.name.localeCompare(second.name, undefined, { sensitivity: "base" }));
  return [{ organizationID: "", name: "Vaults", allVault: store.allVault.filter((vault) => !vault.organizationId) }, ...allOrganizationGroup];
});
const allTeamChoice = computed<TeamChoice[]>(() =>
  store.organizations.organizations.flatMap((organization) =>
    (store.organizationByID[organization.id]?.teams ?? [])
      .filter((team) => team.members.some((member) => member.isCurrentUser))
      .map((team) => ({
        key: `${organization.id}/${team.id}`,
        organizationID: organization.id,
        teamID: team.id,
        teamName: team.name,
        label: `${organization.name} › ${team.name} (team vault)`,
      })),
  ),
);
const selectedTeamChoice = computed(() => allTeamChoice.value.find((choice) => choice.key === newVaultWhere.value));
// canSubmitNewVault: Create needs a name, the loaded teams, and a Where the form lists: the user's own vault or one of the
// teams. A team that is no longer listed (its organization or the user's place in it changed) never falls back to an own vault.
const canSubmitNewVault = computed(
  () => newVaultName.value.trim() !== "" && !isLoadingTeams.value && (newVaultWhere.value === "" || selectedTeamChoice.value !== undefined),
);
// itemMenu is the right-click menu of a list item. detail arrives after the menu opens: list summaries carry no password
// or fields, so the copy actions appear once it has loaded. favorite is set when the item was right-clicked in Favorites.
const itemMenu = ref<{ vaultID: string; vaultItem: ItemDetail; favorite?: FavoriteItem; detail: ItemDetail | null; x: number; y: number } | null>(null);
const itemToDelete = ref<{ vaultID: string; itemID: string; title: string } | null>(null);
const itemToMove = ref<{ vaultID: string; itemID: string; title: string } | null>(null);
// isOverlayOpen: a panel or dialog covers the main window, so its shortcuts and list keys stay out of the way.
const isOverlayOpen = computed(
  () =>
    store.isSettingsOpen ||
    store.isSharingOpen ||
    store.isImportOpen ||
    store.isShareLinksOpen ||
    store.shareLinkDialog !== null ||
    store.vaultDialog !== null ||
    store.browserPairing !== null ||
    store.itemDetailDialog !== null ||
    store.discardDraftPrompt !== null ||
    store.organizationAlert !== null ||
    itemToDelete.value !== null ||
    itemToMove.value !== null,
);

async function openItemMenu(vaultID: string, vaultItem: ItemDetail, event: MouseEvent, favorite?: FavoriteItem): Promise<void> {
  itemMenu.value = { vaultID, vaultItem, favorite, detail: null, x: event.clientX, y: event.clientY };
  try {
    const detail = await api.getItem(vaultID, vaultItem.id);
    if (itemMenu.value?.vaultID === vaultID && itemMenu.value.vaultItem.id === vaultItem.id) {
      itemMenu.value = { ...itemMenu.value, detail };
    }
  } catch {
    return;
  }
}

// buildItemMenuActions lists what applies to the item, in groups split by separators: open, copy and open the website;
// favorite and share link; edit, duplicate, move and delete where its vault is writable.
function buildItemMenuActions(): (ContextMenuAction | null)[] {
  const menu = itemMenu.value;
  if (!menu) {
    return [];
  }
  const { vaultID, vaultItem, favorite } = menu;
  const detail = menu.detail ?? vaultItem;
  const open = async (): Promise<void> => (favorite ? selectFavoriteItem(favorite) : selectItem(vaultItem.id));
  const copy = (id: string, label: string, fieldName: string, value: string | undefined | boolean): ContextMenuAction | null =>
    value ? { id, label: `Copy ${label.toLowerCase()}`, icon: "copy", run: () => copyItemField(vaultID, vaultItem.id, fieldName, label) } : null;
  const link = vaultItem.urls[0];
  const firstGroup = [
    { id: "open", label: "Open", icon: "external" as const, run: open },
    copy("copy-username", "Username", "username", detail.username),
    copy("copy-password", "Password", "password", detail.password),
    copy("copy-one-time-code", "One-time code", "one-time code", detail.hasOneTimePassword),
    copy("copy-card-number", cardFieldName.number, cardFieldName.number, detail.card?.number),
    copy("copy-license-key", softwareLicenseFieldName.licenseKey, softwareLicenseFieldName.licenseKey, detail.softwareLicense?.licenseKey),
    copy("copy-secret", apiCredentialFieldName.secret, apiCredentialFieldName.secret, detail.apiCredential?.secret),
    copy("copy-connection-string", "Connection string", "connection string", detail.database?.connectionString),
    copy("copy-public-key", "Public key", "public key", detail.publicKey),
    link
      ? { id: "open-website", label: vaultItem.category === serverCategory ? "Open admin console" : "Open website", icon: "globe" as const, run: () => openWebsite(link) }
      : null,
  ];
  // A share link needs the item's values, so it is offered once the details have loaded.
  const shareAction: ContextMenuAction | null =
    menu.detail && hasShareableField(menu.detail)
      ? {
          id: "share-link",
          label: "Share link…",
          icon: "link",
          run: async () => {
            await open();
            if (store.selectedItem?.id === vaultItem.id) {
              await openItemShareLinkDialog();
            }
          },
        }
      : null;
  const favoriteGroup = [
    {
      id: "favorite",
      label: vaultItem.isFavorite ? "Remove from Favorites" : "Add to Favorites",
      icon: vaultItem.isFavorite ? ("starFilled" as const) : ("star" as const),
      run: () => setItemFavorite(vaultID, vaultItem.id, !vaultItem.isFavorite),
    },
    shareAction,
  ];
  const vault = store.allVault.find((candidate) => candidate.id === vaultID);
  const isWritable = vault !== undefined && vault.role !== "reader";
  const hasMoveTarget = store.allVault.some((candidate) => candidate.id !== vaultID && candidate.role !== "reader");
  const target = { vaultID, itemID: vaultItem.id, title: vaultItem.title };
  const editGroup = isWritable
    ? [
        {
          id: "edit",
          label: "Edit",
          icon: "edit" as const,
          run: async () => {
            await open();
            if (store.selectedItem?.id === vaultItem.id) {
              startEditingItem();
            }
          },
        },
        vaultItem.category !== sshKeyCategory ? { id: "duplicate", label: "Duplicate", icon: "duplicate" as const, run: () => duplicateItem(vaultID, vaultItem.id) } : null,
        hasMoveTarget ? { id: "move", label: "Move to…", icon: "move" as const, run: () => (itemToMove.value = target) } : null,
        { id: "delete", label: "Delete…", icon: "trash" as const, isDanger: true, run: () => (itemToDelete.value = target) },
      ]
    : [];
  return [firstGroup, favoriteGroup, editGroup]
    .map((group) => group.filter((action): action is ContextMenuAction => action !== null))
    .filter((group) => group.length > 0)
    .flatMap((group, index) => (index === 0 ? group : [null, ...group]));
}
const allItemMenuAction = computed(buildItemMenuActions);

function getItemSubtitle(vaultItem: ItemDetail): string {
  if (vaultItem.category === sshKeyCategory) {
    return `${formatKeyType(vaultItem.keyType)} SSH key`;
  }
  if (vaultItem.category === secureNoteCategory) {
    return "Secure note";
  }
  if (vaultItem.category === creditCardCategory) {
    return formatCardSummary(vaultItem.card);
  }
  if (vaultItem.category === identityCategory) {
    return formatIdentitySummary(vaultItem.identity);
  }
  if (vaultItem.category === apiCredentialCategory) {
    return formatAPICredentialSummary(vaultItem.apiCredential);
  }
  if (vaultItem.category === addressCategory) {
    return formatAddressSummary(vaultItem.address);
  }
  if (vaultItem.category === softwareLicenseCategory) {
    return formatSoftwareLicenseSummary(vaultItem.softwareLicense);
  }
  if (vaultItem.category === serverCategory) {
    return formatServerSummary(vaultItem.server, vaultItem.username);
  }
  if (vaultItem.category === databaseCategory) {
    return formatDatabaseSummary(vaultItem.database);
  }
  return vaultItem.username || (vaultItem.urls[0] ? getDomain(vaultItem.urls[0]) : "");
}

function isSelectedFavorite(favorite: FavoriteItem): boolean {
  return store.selectedItem?.id === favorite.item.id && store.selectedVaultID === favorite.vaultId;
}
const vaultMenu = ref<{ vaultID: string; x: number; y: number } | null>(null);
const vaultMenuVault = computed(() => store.allVault.find((vault) => vault.id === vaultMenu.value?.vaultID));

function openVaultMenuAtPointer(vaultID: string, event: MouseEvent): void {
  vaultMenu.value = { vaultID, x: event.clientX, y: event.clientY };
}

function openVaultMenuBelow(event: MouseEvent): void {
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect();
  vaultMenu.value = { vaultID: store.selectedVaultID, x: Math.max(8, bounds.right - 168), y: bounds.bottom + 6 };
}

// prepareNewVaultForm clears the form and loads the teams for Where whenever the new-vault form opens, from the New vault
// button or from Settings → People.
async function prepareNewVaultForm(): Promise<void> {
  newVaultWhere.value = "";
  newVaultErrorMessage.value = "";
  isLoadingTeams.value = true;
  const load = ++newVaultTeamLoad;
  await loadAllOrganizationQuietly();
  if (load === newVaultTeamLoad) {
    isLoadingTeams.value = false;
  }
}

watch(
  () => store.isCreatingVault,
  (isCreating) => isCreating && prepareNewVaultForm(),
  { immediate: true },
);

async function submitNewVault(): Promise<void> {
  if (!canSubmitNewVault.value) {
    return;
  }
  const team = selectedTeamChoice.value;
  const isCreated = team ? await createOrganizationVault(team.organizationID, team.teamID, newVaultName.value) : await createVault(newVaultName.value);
  if (isCreated) {
    newVaultName.value = "";
    store.isCreatingVault = false;
  }
  newVaultErrorMessage.value = isCreated ? "" : takeErrorMessage();
}

function chooseNewItem(create: () => void): void {
  isNewItemMenuOpen.value = false;
  create();
}

function closeNewItemMenu(event: Event): void {
  if (!isNewItemMenuOpen.value) {
    return;
  }
  if (event instanceof KeyboardEvent ? event.key === "Escape" : !newItemMenu.value?.contains(event.target as Node)) {
    isNewItemMenuOpen.value = false;
  }
}

// listRows are the rows of the list on screen, in the order of allVisibleItem or allVisibleFavorite.
function listRows(): HTMLElement[] {
  return [...(itemList.value?.querySelectorAll<HTMLElement>('[role="option"]') ?? [])];
}

function selectRow(index: number): Promise<void> {
  return store.isFavoritesSelected ? selectFavoriteItem(allVisibleFavorite.value[index]) : selectItem(allVisibleItem.value[index].id);
}

// focusRow moves the keyboard focus and the selection to the row; the search field is the row above the first.
function focusRow(index: number): void {
  const rows = listRows();
  if (index < 0) {
    searchInput.value?.focus();
    return;
  }
  if (index >= rows.length) {
    return;
  }
  rows[index].focus();
}

function selectFocusedRow(index: number): void {
  if (listRows()[index]?.getAttribute("aria-selected") !== "true") {
    void selectRow(index);
  }
}

function enterList(): void {
  focusRow(Math.max(0, listRows().findIndex((row) => row.getAttribute("aria-selected") === "true")));
}

async function openRowInEditor(index: number): Promise<void> {
  const item = store.isFavoritesSelected ? allVisibleFavorite.value[index]?.item : allVisibleItem.value[index];
  if (!item) {
    return;
  }
  await selectRow(index);
  if (store.selectedItem?.id === item.id && store.editorMode === "view" && canEdit.value) {
    startEditingItem();
  }
}

function handleListKey(event: KeyboardEvent): void {
  if (event.metaKey || event.ctrlKey || event.altKey) {
    return;
  }
  const rows = listRows();
  const current = rows.indexOf(document.activeElement as HTMLElement);
  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    event.preventDefault();
    const from = current >= 0 ? current : rows.findIndex((row) => row.getAttribute("aria-selected") === "true");
    const next = event.key === "ArrowDown" ? Math.min(from + 1, rows.length - 1) : from - 1;
    if (next !== current) {
      focusRow(next);
    }
  } else if (event.key === "Enter" && current >= 0) {
    event.preventDefault();
    void openRowInEditor(current);
  }
}

function copySelectedField(fieldName: "username" | "password", label: string): void {
  if (store.editorMode !== "view") {
    return;
  }
  const index = listRows().indexOf(document.activeElement as HTMLElement);
  const favorite = store.isFavoritesSelected ? allVisibleFavorite.value[index] : undefined;
  const selected = index >= 0 ? favorite?.item ?? allVisibleItem.value[index] : store.selectedItem;
  const vaultID = favorite?.vaultId ?? store.selectedVaultID;
  if (selected && (index >= 0 || selected[fieldName])) {
    void copyItemField(vaultID, selected.id, fieldName, label);
  }
}

// moveVaultSelection steps through the sidebar's rows: Favorites, then the vaults as listed.
function moveVaultSelection(step: number): void {
  const allVaultID = ["", ...allVaultGroup.value.flatMap((group) => group.allVault.map((vault) => vault.id))];
  const current = store.isFavoritesSelected ? 0 : allVaultID.indexOf(store.selectedVaultID);
  const next = Math.min(allVaultID.length - 1, Math.max(0, current + step));
  if (next !== current) {
    void (next === 0 ? selectFavorites() : selectVault(allVaultID[next]));
  }
}

function handleShortcut(event: KeyboardEvent): void {
  if (event.altKey && !event.metaKey && !event.ctrlKey && (event.key === "ArrowDown" || event.key === "ArrowUp")) {
    if (store.editorMode === "view" && !isOverlayOpen.value) {
      event.preventDefault();
      moveVaultSelection(event.key === "ArrowDown" ? 1 : -1);
    }
    return;
  }
  if (!(event.metaKey || event.ctrlKey) || event.altKey) {
    return;
  }
  const key = event.key.toLowerCase();
  if (lockOnShortcut(event)) {
    return;
  }
  if (isOverlayOpen.value) {
    return;
  }
  if (event.shiftKey) {
    if (key === "c") {
      event.preventDefault();
      copySelectedField("password", "Password");
    }
    return;
  }
  if (key === "f") {
    event.preventDefault();
    searchInput.value?.focus();
  } else if (key === "n" && canCreate.value) {
    event.preventDefault();
    startCreatingItem();
  } else if (key === "c" && itemList.value?.contains(document.activeElement)) {
    event.preventDefault();
    copySelectedField("username", "Username");
  } else if (key === "d" && store.selectedItem && store.editorMode === "view") {
    event.preventDefault();
    toggleSelectedItemFavorite();
  } else if (key === ",") {
    event.preventDefault();
    openSettings();
  }
}

let syncTimer: ReturnType<typeof setInterval> | undefined;

onMounted(() => {
  window.addEventListener("keydown", handleShortcut);
  window.addEventListener("keydown", closeNewItemMenu);
  window.addEventListener("pointerdown", closeNewItemMenu);
  window.addEventListener("focus", syncInBackground);
  syncTimer = setInterval(syncInBackground, syncInterval);
});
onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleShortcut);
  window.removeEventListener("keydown", closeNewItemMenu);
  window.removeEventListener("pointerdown", closeNewItemMenu);
  window.removeEventListener("focus", syncInBackground);
  clearInterval(syncTimer);
});
</script>

<template>
  <div class="layout" :style="layoutStyle">
    <aside class="sidebar">
      <div class="title-bar"></div>
      <div class="sidebar-brand"><HisarLogo :size="24" show-wordmark /></div>
      <OrganizationBanner />
      <RemovedVaultBanner />

      <nav aria-label="Favorites" class="favorites-nav">
        <button
          class="vault"
          :class="{ selected: store.isFavoritesSelected }"
          :aria-current="store.isFavoritesSelected ? 'true' : undefined"
          data-testid="open-favorites"
          @click="selectFavorites"
        >
          <AppIcon name="star" />
          <span class="vault-name">Favorites</span>
          <span v-if="store.allFavorite.length > 0" class="vault-count" data-testid="favorites-count">{{ store.allFavorite.length }}</span>
        </button>
      </nav>
      <nav v-if="inbox.entryCount > 0" aria-label="Inbox" class="inbox-nav">
        <button class="vault" data-testid="open-inbox" @click="openSettings(inbox.sectionID)">
          <AppIcon name="mail" />
          <span class="vault-name">Inbox</span>
          <span v-if="inbox.actionCount > 0" class="inbox-badge" data-testid="inbox-count" :aria-label="`${inbox.actionCount} waiting for you`">{{ inbox.actionCount }}</span>
        </button>
      </nav>

      <nav aria-label="Vaults">
        <div
          v-for="group in allVaultGroup"
          :key="group.organizationID"
          class="vault-group"
          :role="group.organizationID ? 'group' : undefined"
          :aria-label="group.organizationID ? group.name : undefined"
          :data-testid="group.organizationID ? `organization-vaults-${group.organizationID}` : 'own-vaults'"
        >
          <p class="section-label">{{ group.name }}</p>
          <button
            v-for="vault in group.allVault"
            :key="vault.id"
            class="vault"
            :class="{ selected: !store.isFavoritesSelected && vault.id === store.selectedVaultID }"
            :aria-current="!store.isFavoritesSelected && vault.id === store.selectedVaultID ? 'true' : undefined"
            :data-testid="`vault-${vault.id}`"
            @click="selectVault(vault.id)"
            @contextmenu.prevent="openVaultMenuAtPointer(vault.id, $event)"
          >
            <AppIcon :name="vault.type === 'personal' ? 'user' : 'users'" />
            <span class="vault-name">{{ vault.name }}</span>
            <span class="vault-count">{{ vault.readIssues?.length ? `${vault.itemCount} + ${vault.readIssues.length} unreadable` : vault.itemCount }}</span>
          </button>
        </div>
        <form v-if="store.isCreatingVault" class="new-vault" data-testid="new-vault-form" @submit.prevent="submitNewVault">
          <input v-model="newVaultName" name="vaultName" placeholder="Vault name" autofocus />
          <label v-if="allTeamChoice.length > 0 || newVaultWhere !== ''" class="new-vault-where">
            <span>Where</span>
            <select v-model="newVaultWhere" name="vaultWhere" data-testid="new-vault-where">
              <option value="">Your own vault (share it later)</option>
              <option v-for="choice in allTeamChoice" :key="choice.key" :value="choice.key">{{ choice.label }}</option>
            </select>
          </label>
          <p v-if="selectedTeamChoice" class="muted new-vault-note" data-testid="new-vault-where-note">
            {{ selectedTeamChoice.teamName }}'s members get this vault. Manage it in Settings → Organizations.
          </p>
          <p v-if="newVaultErrorMessage" class="error new-vault-error" role="alert" data-testid="new-vault-error">{{ newVaultErrorMessage }}</p>
          <div class="row">
            <button type="button" class="ghost" @click="store.isCreatingVault = false">Cancel</button>
            <button class="primary" type="submit" data-testid="create-vault" :disabled="store.isBusy || !canSubmitNewVault">Create</button>
          </div>
        </form>
        <button v-else class="ghost new-vault-button" data-testid="new-vault" @click="store.isCreatingVault = true"><AppIcon name="plus" /> New vault</button>
      </nav>

      <nav v-if="allTag.length > 0" aria-label="Tags" class="tags-nav" data-testid="tag-list">
        <p class="section-label">Tags</p>
        <button
          v-for="tag in allTag"
          :key="tag.name"
          class="vault"
          :class="{ selected: tag.name === store.selectedTag }"
          :aria-pressed="tag.name === store.selectedTag"
          :data-testid="`tag-${tag.name}`"
          @click="selectTag(tag.name)"
        >
          <AppIcon name="tag" />
          <span class="vault-name">{{ tag.name }}</span>
          <span class="vault-count">{{ tag.itemCount }}</span>
        </button>
      </nav>

      <nav aria-label="Sharing" class="sharing-nav">
        <button class="vault" :class="{ selected: store.isShareLinksOpen }" data-testid="open-shared-links" @click="openShareLinks">
          <AppIcon name="link" />
          <span class="vault-name">Shared links</span>
          <span v-if="store.allShareLink.length > 0" class="vault-count">{{ store.allShareLink.length }}</span>
        </button>
      </nav>

      <div class="spacer"></div>
      <div v-if="store.status.isOffline" class="offline" role="status" data-testid="offline-notice">
        <AppIcon name="offline" />
        <div>
          <strong>Offline</strong>
          <p class="muted">Showing the copy saved on this device. Changes can be saved once Hisar reconnects.</p>
        </div>
      </div>
      <div class="account">
        <span class="avatar">{{ getInitials(store.status.name || store.status.email) }}</span>
        <div class="account-text">
          <strong>{{ store.status.name || store.status.email }}</strong>
          <span class="muted">{{ store.status.email }}</span>
        </div>
        <button class="ghost icon-button" :title="`Settings (${shortcutModifier},)`" aria-label="Settings" @click="openSettings()"><AppIcon name="settings" /></button>
        <button class="ghost icon-button" :title="`Lock (${shortcutModifier}L)`" aria-label="Lock" @click="lock"><AppIcon name="lock" /></button>
      </div>
    </aside>
    <ColumnResizer v-model="columnWidths.sidebar" column="sidebar" label="Sidebar width" @commit="saveColumnWidths(columnWidths)" />

    <section class="list">
      <ReadIssuesPanel />
      <div v-if="store.isFavoritesSelected" class="title-bar list-title">
        <div>
          <h2>Favorites</h2>
          <span class="muted">{{ store.allFavorite.length }} {{ store.allFavorite.length === 1 ? "item" : "items" }} · only you see them</span>
        </div>
      </div>
      <div v-else class="title-bar list-title">
        <div>
          <h2>{{ selectedVault?.name ?? "" }}</h2>
          <span class="muted">{{ healthyItemCount }} {{ healthyItemCount === 1 ? "item" : "items" }}<template v-if="unreadableItemCount"> · {{ unreadableItemCount }} unreadable</template></span>
        </div>
        <div ref="newItemMenu" class="new-item">
          <button
            v-if="canShareVault"
            class="share-button"
            title="Share this vault"
            data-testid="share-vault"
            @click="openSharing"
          >
            <AppIcon name="users" :size="14" /> Share
          </button>
          <span v-else-if="selectedVault" class="private-badge" :title="personalVaultNotice" data-testid="personal-vault-private">
            <AppIcon name="lock" :size="13" /> Private
          </span>
          <button
            v-if="selectedVault"
            class="ghost icon-button"
            title="Vault actions"
            aria-label="Vault actions"
            aria-haspopup="menu"
            data-testid="vault-actions"
            @pointerdown.stop
            @click="openVaultMenuBelow"
          >
            <AppIcon name="more" />
          </button>
          <button
            class="primary icon-button"
            :disabled="!canEdit"
            :title="`New item (${shortcutModifier}N for a login)`"
            aria-label="New item"
            aria-haspopup="menu"
            :aria-expanded="isNewItemMenuOpen"
            @click="isNewItemMenuOpen = !isNewItemMenuOpen"
          >
            <AppIcon name="plus" />
          </button>
          <div v-if="isNewItemMenuOpen" class="card new-item-menu" role="menu" aria-label="New item">
            <button class="ghost" role="menuitem" data-testid="new-login" @click="chooseNewItem(startCreatingItem)">
              <AppIcon name="globe" /> Login <kbd>{{ shortcutModifier }}N</kbd>
            </button>
            <button class="ghost" role="menuitem" data-testid="new-secure-note" @click="chooseNewItem(startCreatingSecureNote)">
              <AppIcon name="note" /> Secure note
            </button>
            <button class="ghost" role="menuitem" data-testid="new-credit-card" @click="chooseNewItem(startCreatingCard)">
              <AppIcon name="card" /> Credit card
            </button>
            <button class="ghost" role="menuitem" data-testid="new-identity" @click="chooseNewItem(startCreatingIdentity)">
              <AppIcon name="identity" /> Identity
            </button>
            <button class="ghost" role="menuitem" data-testid="new-api-credential" @click="chooseNewItem(startCreatingAPICredential)">
              <AppIcon name="apiKey" /> API credential
            </button>
            <button class="ghost" role="menuitem" data-testid="new-address" @click="chooseNewItem(startCreatingAddress)">
              <AppIcon name="address" /> Address
            </button>
            <button class="ghost" role="menuitem" data-testid="new-software-license" @click="chooseNewItem(startCreatingSoftwareLicense)">
              <AppIcon name="license" /> Software license
            </button>
            <button class="ghost" role="menuitem" data-testid="new-server" @click="chooseNewItem(startCreatingServer)">
              <AppIcon name="server" /> Server
            </button>
            <button class="ghost" role="menuitem" data-testid="new-database" @click="chooseNewItem(startCreatingDatabase)">
              <AppIcon name="database" /> Database
            </button>
            <button class="ghost" role="menuitem" data-testid="new-ssh-key" @click="chooseNewItem(startCreatingSSHKey)">
              <AppIcon name="key" /> SSH key
            </button>
            <hr class="new-item-separator" />
            <button class="ghost" role="menuitem" data-testid="new-import" @click="chooseNewItem(openImport)">
              <AppIcon name="import" /> Import…
            </button>
          </div>
        </div>
      </div>
      <div class="search">
        <AppIcon name="search" />
        <input ref="searchInput" v-model="store.searchQuery" name="search" type="search" placeholder="Search" aria-label="Search items" @keydown.down.prevent="enterList" />
        <kbd>{{ shortcutModifier }}F</kbd>
      </div>
      <div v-if="store.selectedTag && !store.isFavoritesSelected" class="tag-filter" data-testid="tag-filter">
        <AppIcon name="tag" :size="13" />
        <span class="tag-filter-name">{{ store.selectedTag }}</span>
        <button class="ghost icon-button" aria-label="Show all items" title="Show all items" @click="selectTag(store.selectedTag)"><AppIcon name="close" :size="13" /></button>
      </div>

      <template v-if="store.isFavoritesSelected">
        <div v-if="allVisibleFavorite.length === 0" class="list-empty" data-testid="favorites-empty">
          <AppIcon :name="store.searchQuery ? 'search' : 'star'" :size="26" />
          <template v-if="store.searchQuery">
            <p>No favorites match your search.</p>
          </template>
          <template v-else>
            <p><strong>No favorites yet</strong></p>
            <p class="muted">Star an item with <AppIcon name="star" :size="12" /> or {{ shortcutModifier }}D to keep it here, across all your vaults and devices. Only you see your favorites.</p>
          </template>
        </div>
        <ul v-else ref="itemList" role="listbox" aria-label="Favorites" data-testid="favorites-list" @keydown="handleListKey">
          <li v-for="(favorite, index) in allVisibleFavorite" :key="`${favorite.vaultId}/${favorite.item.id}`" role="presentation">
            <button
              class="item"
              :class="{ selected: isSelectedFavorite(favorite) }"
              role="option"
              :aria-selected="isSelectedFavorite(favorite)"
              :tabindex="index === listEntryIndex ? 0 : -1"
              @focus="selectFocusedRow(index)"
              @click="selectFavoriteItem(favorite)"
              @contextmenu.prevent="openItemMenu(favorite.vaultId, favorite.item, $event, favorite)"
            >
              <ItemTile
                :title="favorite.item.title"
                :icon="getItemIcon(favorite.item.category)"
                :item-icon="favorite.item.icon"
                :websiteURL="getWebsiteIconURL(favorite.item)"
              />
              <span class="item-text">
                <span class="item-title">{{ favorite.item.title }}</span>
                <span class="item-subtitle">{{ getItemSubtitle(favorite.item) }}</span>
              </span>
              <span class="item-vault" :title="`In ${favorite.vaultName}`" data-testid="favorite-vault">
                <AppIcon :name="favorite.vaultType === 'personal' ? 'user' : 'users'" :size="11" /> {{ favorite.vaultName }}
              </span>
            </button>
          </li>
        </ul>
      </template>
      <div v-else-if="allVisibleItem.length === 0" class="list-empty">
        <AppIcon :name="store.searchQuery ? 'search' : store.selectedTag ? 'tag' : 'key'" :size="26" />
        <p>{{ emptyListMessage }}</p>
        <button v-if="!store.searchQuery && !store.selectedTag && canEdit" @click="startCreatingItem"><AppIcon name="plus" /> Add the first item</button>
      </div>
      <ul v-else ref="itemList" role="listbox" :aria-label="selectedVault?.name ?? 'Items'" @keydown="handleListKey">
        <li v-for="(vaultItem, index) in allVisibleItem" :key="vaultItem.id" role="presentation">
          <button
            class="item"
            :class="{ selected: store.selectedItem?.id === vaultItem.id }"
            role="option"
            :aria-selected="store.selectedItem?.id === vaultItem.id"
            :tabindex="index === listEntryIndex ? 0 : -1"
            @focus="selectFocusedRow(index)"
            @click="selectItem(vaultItem.id)"
            @contextmenu.prevent="openItemMenu(store.selectedVaultID, vaultItem, $event)"
          >
            <ItemTile :title="vaultItem.title" :icon="getItemIcon(vaultItem.category)" :item-icon="vaultItem.icon" :websiteURL="getWebsiteIconURL(vaultItem)" />
            <span class="item-text">
              <span class="item-title">{{ vaultItem.title }}</span>
              <span class="item-subtitle">{{ getItemSubtitle(vaultItem) }}</span>
            </span>
            <AppIcon v-if="vaultItem.isFavorite" name="starFilled" :size="12" class="item-star" aria-label="Favorite" />
          </button>
        </li>
      </ul>
    </section>
    <ColumnResizer v-model="columnWidths.list" column="list" label="Item list width" @commit="saveColumnWidths(columnWidths)" />

    <main class="detail">
      <div class="title-bar"></div>
      <div class="detail-content">
        <p v-if="store.errorMessage" class="error detail-error" role="alert">
          <span>{{ store.errorMessage }}</span>
          <button class="ghost icon-button" aria-label="Dismiss" title="Dismiss" data-testid="dismiss-error" @click="clearError"><AppIcon name="close" :size="13" /></button>
        </p>
        <p v-if="store.status.biometricSetupError" class="error" role="status" data-testid="biometric-setup-error">{{ store.status.biometricSetupError }}</p>
        <SSHKeyForm v-if="store.editorMode === 'create-ssh-key'" />
        <ItemEditor v-else-if="store.editorMode !== 'view'" :key="store.editorMode + (store.selectedItem?.id ?? '')" />
        <ItemDetailPanel v-else-if="store.selectedItem" :can-edit="canEdit" :vault-name="selectedVault?.name ?? ''" :vault-type="selectedVault?.type ?? ''" />
        <div v-else class="detail-empty">
          <span class="detail-empty-icon"><AppIcon name="vault" :size="28" /></span>
          <h3>{{ store.isFavoritesSelected ? "Favorites" : selectedVault ? selectedVault.name : "Hisar" }}</h3>
          <p class="muted">Select an item to see its details.</p>
          <dl class="shortcut-list">
            <div><dt><kbd>{{ shortcutModifier }}N</kbd></dt><dd>New item</dd></div>
            <div><dt><kbd>{{ shortcutModifier }}F</kbd></dt><dd>Search</dd></div>
            <div><dt><kbd>{{ shortcutModifier }}C</kbd></dt><dd>Copy username</dd></div>
            <div><dt><kbd>{{ shortcutModifier }}{{ shiftModifier }}C</kbd></dt><dd>Copy password</dd></div>
            <div><dt><kbd>{{ shortcutModifier }}D</kbd></dt><dd>Add to Favorites</dd></div>
            <div><dt><kbd>{{ shortcutModifier }}L</kbd></dt><dd>Lock Hisar</dd></div>
          </dl>
        </div>
      </div>
    </main>
    <SettingsPanel v-if="store.isSettingsOpen" />
    <VaultSharingPanel v-if="store.isSharingOpen" />
    <ImportPanel v-if="store.isImportOpen" />
    <SharedLinksPanel v-if="store.isShareLinksOpen" />
    <VaultDialog v-if="store.vaultDialog" :key="`${store.vaultDialog.mode}-${store.vaultDialog.actions.vault.id}`" :dialog="store.vaultDialog" />
    <VaultMenu v-if="vaultMenu && vaultMenuVault" :vault="vaultMenuVault" :x="vaultMenu.x" :y="vaultMenu.y" @close="vaultMenu = null" />
    <ContextMenu
      v-if="itemMenu"
      :key="`${itemMenu.vaultID}/${itemMenu.vaultItem.id}`"
      :label="itemMenu.vaultItem.title"
      :x="itemMenu.x"
      :y="itemMenu.y"
      :all-action="allItemMenuAction"
      @close="itemMenu = null"
    />
    <MoveItemDialog v-if="itemToMove" :vault-i-d="itemToMove.vaultID" :item-i-d="itemToMove.itemID" :title="itemToMove.title" @close="itemToMove = null" />
    <ConfirmDeleteDialog v-if="itemToDelete" :vault-i-d="itemToDelete.vaultID" :item-i-d="itemToDelete.itemID" :title="itemToDelete.title" @close="itemToDelete = null" />
    <DiscardChangesDialog v-if="store.discardDraftPrompt" :prompt="store.discardDraftPrompt" />
    <ShareLinkDialog v-if="store.shareLinkDialog" :key="`${store.shareLinkDialog.mode}-${store.shareLinkDialog.itemID}`" />
    <OrganizationAlert />
  </div>
</template>

<style scoped>
.layout {
  display: grid;
  height: 100%;
}

.sidebar {
  display: flex;
  flex-direction: column;
  padding: 0 10px 10px;
  border-right: 1px solid var(--color-border);
  background: var(--color-sidebar);
  overflow-y: auto;
}

.sidebar-brand {
  padding: 0 8px 18px;
}

.section-label {
  padding: 0 8px;
}

.vault {
  width: 100%;
  justify-content: flex-start;
  gap: 10px;
  margin-bottom: 1px;
  padding: 7px 8px;
  border: none;
  background: none;
  color: var(--color-text-secondary);
  font-weight: 500;
}

.vault:hover:not(:disabled) {
  background: var(--color-hover);
  border: none;
}

.vault.selected {
  background: var(--color-selected);
  color: var(--color-text);
}

.vault.selected > :deep(svg) {
  color: var(--color-accent-text);
}

.vault-name {
  flex: 1;
  overflow: hidden;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.vault-count {
  min-width: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: var(--color-surface-raised);
  font-size: var(--font-size-11);
  color: var(--color-text-muted);
}

.new-vault-button {
  width: 100%;
  justify-content: flex-start;
  gap: 10px;
  margin-top: 4px;
  padding: 7px 8px;
  color: var(--color-text-muted);
}

.new-vault {
  margin: 6px 0;
}

.sharing-nav,
.tags-nav {
  margin-top: 14px;
}

.tag-filter {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: -4px 14px 10px;
  padding: 3px 4px 3px 10px;
  border: 1px solid var(--color-border-strong);
  border-radius: 14px;
  font-size: var(--font-size-12);
  color: var(--color-text-secondary);
}

.tag-filter-name {
  flex: 1;
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tag-filter .icon-button {
  padding: 2px;
}

.favorites-nav {
  margin-bottom: 14px;
}

.inbox-nav {
  margin: -13px 0 14px;
}

.inbox-badge {
  min-width: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--color-primary);
  color: var(--color-primary-text);
  font-size: var(--font-size-11);
  font-weight: 600;
  line-height: 18px;
  text-align: center;
}

.item-star {
  margin-left: auto;
  color: var(--color-warning);
}

.item-vault {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: none;
  max-width: 96px;
  margin-left: auto;
  padding: 1px 6px;
  overflow: hidden;
  border-radius: 8px;
  background: var(--color-surface-raised);
  font-size: var(--font-size-11);
  color: var(--color-text-muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.new-vault input {
  margin-bottom: 6px;
}

.new-vault-where {
  margin-bottom: 6px;
}

.new-vault-where span {
  padding: 0 2px;
}

.new-vault-where select {
  width: 100%;
}

.new-vault-note {
  margin: 0 2px 8px;
  font-size: var(--font-size-11);
  line-height: 1.4;
}

.new-vault-error {
  margin: 0 0 8px;
  padding: 6px 8px;
  font-size: var(--font-size-12);
}

.new-vault .row {
  justify-content: flex-end;
}

.vault-group + .vault-group {
  margin-top: 14px;
}

.offline {
  display: flex;
  gap: 10px;
  margin: 0 8px 10px;
  padding: 10px 12px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-warning);
}

.offline strong {
  font-size: var(--font-size-13);
  color: var(--color-text);
}

.offline p {
  margin: 2px 0 0;
  font-size: var(--font-size-12);
  line-height: 1.4;
}

.account {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 8px;
  border-top: 1px solid var(--color-border);
}

.avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  flex: none;
  border-radius: 50%;
  background: var(--color-selected);
  font-size: var(--font-size-11);
  font-weight: 600;
}

.account-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.account-text strong,
.account-text span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-text span {
  font-size: var(--font-size-11);
}

.list {
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--color-border);
  background: var(--color-panel);
  overflow: hidden;
}

.list-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: auto;
  min-height: var(--title-bar-height);
  padding: 14px 14px 10px;
}

.list-title h2 {
  margin: 0;
  font-size: var(--font-size-15);
  font-weight: 600;
}

.list-title .muted {
  font-size: var(--font-size-12);
}

.new-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 6px;
  --wails-draggable: no-drag;
}

.share-button,
.private-badge {
  gap: 5px;
  padding: 5px 9px;
  font-size: var(--font-size-12);
}

.private-badge {
  display: inline-flex;
  align-items: center;
  color: var(--color-text-muted);
  cursor: default;
}

.new-item-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  z-index: 5;
  display: flex;
  flex-direction: column;
  min-width: 168px;
  padding: 4px;
  background: var(--color-surface-raised);
  box-shadow: 0 12px 32px var(--color-shadow);
}

.new-item-menu button {
  justify-content: flex-start;
  gap: 10px;
  padding: 7px 8px;
  color: var(--color-text);
}

.new-item-menu kbd {
  margin-left: auto;
}

.new-item-separator {
  width: 100%;
  margin: 4px 0;
  border: none;
  border-top: 1px solid var(--color-border);
}

.search {
  position: relative;
  margin: 0 14px 10px;
  color: var(--color-text-muted);
}

.search > :deep(.icon) {
  position: absolute;
  top: 50%;
  left: 10px;
  transform: translateY(-50%);
}

.search input {
  padding: 7px 44px 7px 32px;
  color: var(--color-text);
}

.search kbd {
  position: absolute;
  top: 50%;
  right: 8px;
  transform: translateY(-50%);
}

.list ul {
  flex: 1;
  margin: 0;
  padding: 0 8px 8px;
  list-style: none;
  overflow-y: auto;
}

.item {
  width: 100%;
  justify-content: flex-start;
  gap: 12px;
  padding: 8px;
  border: none;
  background: none;
  text-align: left;
}

.item:hover:not(:disabled) {
  background: var(--color-hover);
  border: none;
}

.item.selected {
  background: var(--color-selected);
}

.item.selected .item-subtitle {
  color: var(--color-text-secondary);
}

.item-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.item-title {
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-subtitle {
  overflow: hidden;
  font-size: var(--font-size-12);
  color: var(--color-text-muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.list-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  margin-top: 48px;
  padding: 0 24px;
  color: var(--color-text-muted);
  text-align: center;
}

.list-empty p {
  margin: 0;
}

.detail {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  container: detail / inline-size;
}

.detail-content {
  flex: 1;
  padding: 4px 32px 32px;
  overflow-y: auto;
}

.detail-error {
  align-items: center;
  justify-content: space-between;
}

.detail-error .icon-button {
  flex: none;
  margin: -4px -6px -4px 0;
  color: inherit;
}

.detail-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin-top: 12vh;
  text-align: center;
}

.detail-empty-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 60px;
  height: 60px;
  border: 1px solid var(--color-border);
  border-radius: 16px;
  background: var(--color-surface);
  color: var(--color-text-muted);
}

.detail-empty h3 {
  margin: 16px 0 4px;
  font-size: var(--font-size-15);
}

.detail-empty p {
  margin: 0 0 24px;
}

.shortcut-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  color: var(--color-text-muted);
}

.shortcut-list div {
  display: grid;
  grid-template-columns: 56px 1fr;
  align-items: center;
  gap: 8px;
  text-align: left;
}

.shortcut-list dt {
  text-align: right;
}

.shortcut-list dd {
  margin: 0;
}
</style>
