<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { createVault, filterAllItem, lock, openSharing, openVaultRecovery, selectItem, selectVault, startCreatingItem, startCreatingSSHKey, store, syncInBackground, syncInterval } from "../store";
import { formatKeyType, getDomain, getInitials, isMacPlatform, sshKeyCategory } from "../util";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";
import ItemDetailPanel from "./ItemDetailPanel.vue";
import ItemEditor from "./ItemEditor.vue";
import ItemTile from "./ItemTile.vue";
import SettingsPanel from "./SettingsPanel.vue";
import SSHKeyForm from "./SSHKeyForm.vue";
import VaultRecoveryPanel from "./VaultRecoveryPanel.vue";
import VaultSharingPanel from "./VaultSharingPanel.vue";

const personalVaultNotice = "Personal vaults are private. Create a shared vault to share with your family. Click to see who can recover this vault.";

const isCreatingVault = ref(false);
const newVaultName = ref("");
const searchInput = ref<HTMLInputElement | null>(null);
const isNewItemMenuOpen = ref(false);
const newItemMenu = ref<HTMLElement | null>(null);
const shortcutModifier = isMacPlatform() ? "⌘" : "Ctrl+";

const allVisibleItem = computed(() => filterAllItem(store.allItem, store.searchQuery));
const selectedVault = computed(() => store.allVault.find((vault) => vault.id === store.selectedVaultID));
const canEdit = computed(() => selectedVault.value !== undefined && selectedVault.value.role !== "reader");

async function submitNewVault(): Promise<void> {
  if (await createVault(newVaultName.value)) {
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
  if (key === "f") {
    event.preventDefault();
    searchInput.value?.focus();
  } else if (key === "n" && canEdit.value) {
    event.preventDefault();
    startCreatingItem();
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

      <nav aria-label="Vaults">
        <p class="section-label">Vaults</p>
        <button
          v-for="vault in store.allVault"
          :key="vault.id"
          class="vault"
          :class="{ selected: vault.id === store.selectedVaultID }"
          @click="selectVault(vault.id)"
        >
          <AppIcon :name="vault.type === 'personal' ? 'user' : 'users'" />
          <span class="vault-name">{{ vault.name }}</span>
          <span class="vault-count">{{ vault.itemCount }}</span>
        </button>
        <form v-if="isCreatingVault" class="new-vault" @submit.prevent="submitNewVault">
          <input v-model="newVaultName" name="vaultName" placeholder="Vault name" autofocus />
          <div class="row">
            <button type="button" class="ghost" @click="isCreatingVault = false">Cancel</button>
            <button class="primary" type="submit" :disabled="store.isBusy || !newVaultName.trim()">Create</button>
          </div>
        </form>
        <button v-else class="ghost new-vault-button" @click="isCreatingVault = true"><AppIcon name="plus" /> New vault</button>
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
      <div class="title-bar list-title">
        <div>
          <h2>{{ selectedVault?.name ?? "" }}</h2>
          <span class="muted">{{ store.allItem.length }} {{ store.allItem.length === 1 ? "item" : "items" }}</span>
        </div>
        <div ref="newItemMenu" class="new-item">
          <button
            v-if="selectedVault?.type === 'shared'"
            class="share-button"
            title="Share this vault"
            data-testid="share-vault"
            @click="openSharing"
          >
            <AppIcon name="users" :size="14" /> Share
          </button>
          <button
            v-else-if="selectedVault"
            class="ghost private-button"
            :title="personalVaultNotice"
            data-testid="personal-vault-private"
            @click="openVaultRecovery"
          >
            <AppIcon name="lock" :size="13" /> Private
            <AppIcon v-if="selectedVault.recoveryOptOut" name="warning" :size="13" class="warning-text" data-testid="vault-recovery-off" />
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
            <button class="ghost" role="menuitem" data-testid="new-ssh-key" @click="chooseNewItem(startCreatingSSHKey)">
              <AppIcon name="key" /> SSH key
            </button>
          </div>
        </div>
      </div>
      <div class="search">
        <AppIcon name="search" />
        <input ref="searchInput" v-model="store.searchQuery" name="search" type="search" placeholder="Search" aria-label="Search items" />
        <kbd>{{ shortcutModifier }}F</kbd>
      </div>

      <div v-if="allVisibleItem.length === 0" class="list-empty">
        <AppIcon :name="store.searchQuery ? 'search' : 'key'" :size="26" />
        <p>{{ store.searchQuery ? "No items match your search." : "This vault is empty." }}</p>
        <button v-if="!store.searchQuery && canEdit" @click="startCreatingItem"><AppIcon name="plus" /> Add the first item</button>
      </div>
      <ul v-else>
        <li v-for="vaultItem in allVisibleItem" :key="vaultItem.id">
          <button class="item" :class="{ selected: store.selectedItem?.id === vaultItem.id }" @click="selectItem(vaultItem.id)">
            <ItemTile :title="vaultItem.title" :icon="vaultItem.category === sshKeyCategory ? 'key' : undefined" />
            <span class="item-text">
              <span class="item-title">{{ vaultItem.title }}</span>
              <span v-if="vaultItem.category === sshKeyCategory" class="item-subtitle">{{ formatKeyType(vaultItem.keyType) }} SSH key</span>
              <span v-else class="item-subtitle">{{ vaultItem.username || (vaultItem.urls[0] ? getDomain(vaultItem.urls[0]) : "") }}</span>
            </span>
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
        <ItemDetailPanel v-else-if="store.selectedItem" :can-edit="canEdit" :vault-name="selectedVault?.name ?? ''" :is-shared="selectedVault?.type === 'shared'" />
        <div v-else class="detail-empty">
          <span class="detail-empty-icon"><AppIcon name="vault" :size="28" /></span>
          <h3>{{ selectedVault ? selectedVault.name : "Hisar" }}</h3>
          <p class="muted">Select an item to see its details.</p>
          <dl class="shortcut-list">
            <div><dt><kbd>{{ shortcutModifier }}N</kbd></dt><dd>New item</dd></div>
            <div><dt><kbd>{{ shortcutModifier }}F</kbd></dt><dd>Search</dd></div>
            <div><dt><kbd>{{ shortcutModifier }}L</kbd></dt><dd>Lock Hisar</dd></div>
          </dl>
        </div>
      </div>
    </main>
    <SettingsPanel v-if="store.isSettingsOpen" />
    <VaultSharingPanel v-if="store.isSharingOpen" />
    <VaultRecoveryPanel v-if="store.isVaultRecoveryOpen" />
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

.new-vault input {
  margin-bottom: 6px;
}

.new-vault .row {
  justify-content: flex-end;
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
.private-button {
  gap: 5px;
  padding: 5px 9px;
  font-size: 12px;
}

.private-button {
  color: var(--color-text-muted);
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
  box-shadow: 0 12px 32px rgb(0 0 0 / 45%);
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
