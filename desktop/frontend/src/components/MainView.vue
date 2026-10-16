<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import {
  createOrganizationVault,
  createVault,
  filterAllFavorite,
  filterAllItem,
  listAllTag,
  loadAllOrganizationQuietly,
  lock,
  openImport,
  openShareLinks,
  openSharing,
  openTextShareLinkDialog,
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
  startCreatingAddress,
  startCreatingSecureNote,
  startCreatingSSHKey,
  store,
  syncInBackground,
  syncInterval,
  toggleSelectedItemFavorite,
} from "../store";
import type { FavoriteItem, ItemDetail, VaultSummary } from "../api";
import { addressCategory, apiCredentialCategory, creditCardCategory, formatAddressSummary, formatAPICredentialSummary, formatCardSummary, formatIdentitySummary, formatKeyType, getDomain, getInitials, getItemIcon, identityCategory, isMacPlatform, secureNoteCategory, sshKeyCategory } from "../util";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";
import ItemDetailPanel from "./ItemDetailPanel.vue";
import ItemEditor from "./ItemEditor.vue";
import ItemMenu from "./ItemMenu.vue";
import ImportPanel from "./ImportPanel.vue";
import ItemTile from "./ItemTile.vue";
import SettingsPanel from "./SettingsPanel.vue";
import OrganizationAlert from "./OrganizationAlert.vue";
import OrganizationBanner from "./OrganizationBanner.vue";
import RemovedVaultBanner from "./RemovedVaultBanner.vue";
import ShareInvitationBanner from "./ShareInvitationBanner.vue";
import SharedLinksPanel from "./SharedLinksPanel.vue";
import ShareLinkDialog from "./ShareLinkDialog.vue";
import SSHKeyForm from "./SSHKeyForm.vue";
import VaultDialog from "./VaultDialog.vue";
import VaultMenu from "./VaultMenu.vue";
import VaultSharingPanel from "./VaultSharingPanel.vue";

const personalVaultNotice = "Personal vaults are private. Create a shared vault to share items with other people.";

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

const isCreatingVault = ref(false);
const newVaultName = ref("");
// newVaultWhere is "" for a shared vault, or the key of the team that gets the new organization vault.
const newVaultWhere = ref("");
const searchInput = ref<HTMLInputElement | null>(null);
const isNewItemMenuOpen = ref(false);
const newItemMenu = ref<HTMLElement | null>(null);
const shortcutModifier = isMacPlatform() ? "⌘" : "Ctrl+";

const allVisibleItem = computed(() => filterAllItem(store.allItem, store.searchQuery, store.selectedTag));
const allVisibleFavorite = computed(() => filterAllFavorite(store.allFavorite, store.searchQuery));
const allTag = computed(() => listAllTag(store.allItem));
const emptyListMessage = computed(() => {
  if (store.searchQuery) {
    return "No items match your search.";
  }
  return store.selectedTag ? `No items are tagged ${store.selectedTag}.` : "This vault is empty.";
});
const selectedVault = computed(() => store.allVault.find((vault) => vault.id === store.selectedVaultID));
const isSharedVault = computed(() => selectedVault.value !== undefined && selectedVault.value.type !== "personal");
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
        label: `${organization.name} › ${team.name}`,
      })),
  ),
);
const selectedTeamChoice = computed(() => allTeamChoice.value.find((choice) => choice.key === newVaultWhere.value));
const itemMenu = ref<{ vaultID: string; vaultItem: ItemDetail; x: number; y: number } | null>(null);

function openItemMenu(vaultID: string, vaultItem: ItemDetail, event: MouseEvent): void {
  itemMenu.value = { vaultID, vaultItem, x: event.clientX, y: event.clientY };
}

function toggleItemMenuFavorite(): void {
  if (itemMenu.value) {
    const { vaultID, vaultItem } = itemMenu.value;
    setItemFavorite(vaultID, vaultItem.id, !vaultItem.isFavorite);
  }
}

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

function startCreatingVault(): void {
  newVaultWhere.value = "";
  isCreatingVault.value = true;
  loadAllOrganizationQuietly();
}

async function submitNewVault(): Promise<void> {
  const team = selectedTeamChoice.value;
  const isCreated = team ? await createOrganizationVault(team.organizationID, team.teamID, newVaultName.value) : await createVault(newVaultName.value);
  if (isCreated) {
    newVaultName.value = "";
    isCreatingVault.value = false;
  }
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

function handleShortcut(event: KeyboardEvent): void {
  if (!(event.metaKey || event.ctrlKey) || event.altKey || event.shiftKey) {
    return;
  }
  const key = event.key.toLowerCase();
  if (key === "f" && store.isSettingsOpen) {
    return;
  }
  if (key === "f") {
    event.preventDefault();
    searchInput.value?.focus();
  } else if (key === "n" && canCreate.value) {
    event.preventDefault();
    startCreatingItem();
  } else if (key === "d" && store.selectedItem && store.editorMode === "view" && !store.isSettingsOpen) {
    event.preventDefault();
    toggleSelectedItemFavorite();
  } else if (key === "l") {
    event.preventDefault();
    lock();
  } else if (key === ",") {
    event.preventDefault();
    store.isSettingsOpen = true;
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
  <div class="layout">
    <aside class="sidebar">
      <div class="title-bar"></div>
      <div class="sidebar-brand"><HisarLogo :size="24" show-wordmark /></div>
      <ShareInvitationBanner />
      <OrganizationBanner />
      <RemovedVaultBanner />

      <nav aria-label="Favorites" class="favorites-nav">
        <button class="vault" :class="{ selected: store.isFavoritesSelected }" data-testid="open-favorites" @click="selectFavorites">
          <AppIcon name="star" />
          <span class="vault-name">Favorites</span>
          <span v-if="store.allFavorite.length > 0" class="vault-count" data-testid="favorites-count">{{ store.allFavorite.length }}</span>
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
            :data-testid="`vault-${vault.id}`"
            @click="selectVault(vault.id)"
            @contextmenu.prevent="openVaultMenuAtPointer(vault.id, $event)"
          >
            <AppIcon :name="vault.type === 'personal' ? 'user' : 'users'" />
            <span class="vault-name">{{ vault.name }}</span>
            <span class="vault-count">{{ vault.itemCount }}</span>
          </button>
        </div>
        <form v-if="isCreatingVault" class="new-vault" data-testid="new-vault-form" @submit.prevent="submitNewVault">
          <input v-model="newVaultName" name="vaultName" placeholder="Vault name" autofocus />
          <label v-if="allTeamChoice.length > 0" class="new-vault-where">
            <span>Where</span>
            <select v-model="newVaultWhere" name="vaultWhere" data-testid="new-vault-where">
              <option value="">Shared vault</option>
              <option v-for="choice in allTeamChoice" :key="choice.key" :value="choice.key">{{ choice.label }}</option>
            </select>
          </label>
          <p v-if="selectedTeamChoice" class="muted new-vault-note" data-testid="new-vault-where-note">
            {{ selectedTeamChoice.teamName }}'s members get this vault. Manage it in Settings → Organizations.
          </p>
          <div class="row">
            <button type="button" class="ghost" @click="isCreatingVault = false">Cancel</button>
            <button class="primary" type="submit" data-testid="create-vault" :disabled="store.isBusy || !newVaultName.trim()">Create</button>
          </div>
        </form>
        <button v-else class="ghost new-vault-button" data-testid="new-vault" @click="startCreatingVault"><AppIcon name="plus" /> New vault</button>
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
        <button class="ghost icon-button" :title="`Settings (${shortcutModifier},)`" aria-label="Settings" @click="store.isSettingsOpen = true"><AppIcon name="settings" /></button>
        <button class="ghost icon-button" :title="`Lock (${shortcutModifier}L)`" aria-label="Lock" @click="lock"><AppIcon name="lock" /></button>
      </div>
    </aside>

    <section class="list">
      <div v-if="store.isFavoritesSelected" class="title-bar list-title">
        <div>
          <h2>Favorites</h2>
          <span class="muted">{{ store.allFavorite.length }} {{ store.allFavorite.length === 1 ? "item" : "items" }} · only you see them</span>
        </div>
      </div>
      <div v-else class="title-bar list-title">
        <div>
          <h2>{{ selectedVault?.name ?? "" }}</h2>
          <span class="muted">{{ store.allItem.length }} {{ store.allItem.length === 1 ? "item" : "items" }}</span>
        </div>
        <div ref="newItemMenu" class="new-item">
          <button
            v-if="isSharedVault"
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
            <button class="ghost" role="menuitem" data-testid="new-ssh-key" @click="chooseNewItem(startCreatingSSHKey)">
              <AppIcon name="key" /> SSH key
            </button>
            <button class="ghost" role="menuitem" data-testid="new-import" @click="chooseNewItem(openImport)">
              <AppIcon name="import" /> Import…
            </button>
            <button class="ghost" role="menuitem" data-testid="new-share-text" @click="chooseNewItem(openTextShareLinkDialog)">
              <AppIcon name="link" /> Share text…
            </button>
          </div>
        </div>
      </div>
      <div class="search">
        <AppIcon name="search" />
        <input ref="searchInput" v-model="store.searchQuery" name="search" type="search" placeholder="Search" aria-label="Search items" />
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
        <ul v-else data-testid="favorites-list">
          <li v-for="favorite in allVisibleFavorite" :key="`${favorite.vaultId}/${favorite.item.id}`">
            <button
              class="item"
              :class="{ selected: isSelectedFavorite(favorite) }"
              @click="selectFavoriteItem(favorite)"
              @contextmenu.prevent="openItemMenu(favorite.vaultId, favorite.item, $event)"
            >
              <ItemTile :title="favorite.item.title" :icon="getItemIcon(favorite.item.category)" />
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
      <ul v-else>
        <li v-for="vaultItem in allVisibleItem" :key="vaultItem.id">
          <button
            class="item"
            :class="{ selected: store.selectedItem?.id === vaultItem.id }"
            @click="selectItem(vaultItem.id)"
            @contextmenu.prevent="openItemMenu(store.selectedVaultID, vaultItem, $event)"
          >
            <ItemTile :title="vaultItem.title" :icon="getItemIcon(vaultItem.category)" />
            <span class="item-text">
              <span class="item-title">{{ vaultItem.title }}</span>
              <span class="item-subtitle">{{ getItemSubtitle(vaultItem) }}</span>
            </span>
            <AppIcon v-if="vaultItem.isFavorite" name="starFilled" :size="12" class="item-star" aria-label="Favorite" />
          </button>
        </li>
      </ul>
    </section>

    <main class="detail">
      <div class="title-bar"></div>
      <div class="detail-content">
        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
        <SSHKeyForm v-if="store.editorMode === 'create-ssh-key'" />
        <ItemEditor v-else-if="store.editorMode !== 'view'" :key="store.editorMode + (store.selectedItem?.id ?? '')" />
        <ItemDetailPanel v-else-if="store.selectedItem" :can-edit="canEdit" :vault-name="selectedVault?.name ?? ''" :is-shared="isSharedVault" />
        <div v-else class="detail-empty">
          <span class="detail-empty-icon"><AppIcon name="vault" :size="28" /></span>
          <h3>{{ store.isFavoritesSelected ? "Favorites" : selectedVault ? selectedVault.name : "Hisar" }}</h3>
          <p class="muted">Select an item to see its details.</p>
          <dl class="shortcut-list">
            <div><dt><kbd>{{ shortcutModifier }}N</kbd></dt><dd>New item</dd></div>
            <div><dt><kbd>{{ shortcutModifier }}F</kbd></dt><dd>Search</dd></div>
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
    <ItemMenu
      v-if="itemMenu"
      :title="itemMenu.vaultItem.title"
      :is-favorite="itemMenu.vaultItem.isFavorite"
      :x="itemMenu.x"
      :y="itemMenu.y"
      @toggle-favorite="toggleItemMenuFavorite"
      @close="itemMenu = null"
    />
    <ShareLinkDialog v-if="store.shareLinkDialog" :key="`${store.shareLinkDialog.mode}-${store.shareLinkDialog.itemID}`" />
    <OrganizationAlert />
  </div>
</template>

<style scoped>
.layout {
  display: grid;
  grid-template-columns: 228px 300px 1fr;
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
  font-size: 11px;
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
  font-size: 12px;
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
  font-size: 11px;
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
  font-size: 11px;
  line-height: 1.4;
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
  font-size: 13px;
  color: var(--color-text);
}

.offline p {
  margin: 2px 0 0;
  font-size: 12px;
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
  font-size: 11px;
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
  font-size: 11px;
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
  font-size: 16px;
  font-weight: 650;
}

.list-title .muted {
  font-size: 12px;
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
  font-size: 12px;
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
  font-size: 12px;
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
}

.detail-content {
  flex: 1;
  padding: 4px 32px 32px;
  overflow-y: auto;
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
  font-size: 16px;
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
