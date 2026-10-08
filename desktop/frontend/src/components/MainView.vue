<script setup lang="ts">
import { computed, ref } from "vue";
import {
  createVault,
  filterAllItem,
  lock,
  selectItem,
  selectVault,
  startCreatingItem,
  store,
} from "../store";
import ItemDetailPanel from "./ItemDetailPanel.vue";
import ItemEditor from "./ItemEditor.vue";

const isCreatingVault = ref(false);
const newVaultName = ref("");

const allVisibleItem = computed(() => filterAllItem(store.allItem, store.searchQuery));
const selectedVault = computed(() => store.allVault.find((vault) => vault.id === store.selectedVaultID));
const canEdit = computed(() => selectedVault.value !== undefined && selectedVault.value.role !== "reader");

async function submitNewVault(): Promise<void> {
  if (await createVault(newVaultName.value)) {
    newVaultName.value = "";
    isCreatingVault.value = false;
  }
}
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="title-bar"></div>
      <nav aria-label="Vaults">
        <p class="section-title">Vaults</p>
        <button
          v-for="vault in store.allVault"
          :key="vault.id"
          class="vault"
          :class="{ selected: vault.id === store.selectedVaultID }"
          @click="selectVault(vault.id)"
        >
          {{ vault.name }}
          <span v-if="vault.type === 'shared'" class="muted badge">shared</span>
        </button>
        <form v-if="isCreatingVault" class="new-vault" @submit.prevent="submitNewVault">
          <input v-model="newVaultName" name="vaultName" placeholder="Vault name" autofocus />
          <div class="row">
            <button type="button" @click="isCreatingVault = false">Cancel</button>
            <button class="primary" type="submit" :disabled="store.isBusy">Create</button>
          </div>
        </form>
        <button v-else class="link new-vault-button" @click="isCreatingVault = true">+ New vault</button>
      </nav>
      <div class="spacer"></div>
      <div class="account">
        <p class="muted account-email">{{ store.status.name || store.status.email }}</p>
        <button @click="lock">Lock</button>
      </div>
    </aside>

    <section class="list">
      <div class="title-bar list-header">
        <input v-model="store.searchQuery" name="search" type="search" placeholder="Search" aria-label="Search items" />
        <button class="primary" :disabled="!canEdit" title="New item" @click="startCreatingItem">+</button>
      </div>
      <p v-if="allVisibleItem.length === 0" class="muted empty">
        {{ store.searchQuery ? "No matching items." : "No items in this vault yet." }}
      </p>
      <ul>
        <li v-for="vaultItem in allVisibleItem" :key="vaultItem.id">
          <button
            class="item"
            :class="{ selected: store.selectedItem?.id === vaultItem.id }"
            @click="selectItem(vaultItem.id)"
          >
            <span class="item-title">{{ vaultItem.title }}</span>
            <span class="muted item-subtitle">{{ vaultItem.username || vaultItem.urls[0] || "" }}</span>
          </button>
        </li>
      </ul>
    </section>

    <main class="detail">
      <div class="title-bar"></div>
      <p v-if="store.errorMessage" class="error detail-error" role="alert">{{ store.errorMessage }}</p>
      <ItemEditor v-if="store.editorMode !== 'view'" :key="store.editorMode + (store.selectedItem?.id ?? '')" />
      <ItemDetailPanel v-else-if="store.selectedItem" :can-edit="canEdit" />
      <p v-else class="muted empty">Select an item, or press + to add one.</p>
    </main>
  </div>
</template>

<style scoped>
.layout {
  display: grid;
  grid-template-columns: 220px 300px 1fr;
  height: 100%;
}

.sidebar {
  display: flex;
  flex-direction: column;
  padding: 0 12px 12px;
  background: var(--color-sidebar);
  border-right: 1px solid var(--color-border);
  overflow-y: auto;
}

.section-title {
  margin: 0 8px 6px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.vault,
.item {
  display: flex;
  width: 100%;
  border: none;
  background: none;
  text-align: left;
}

.vault {
  justify-content: space-between;
  align-items: center;
  padding: 6px 8px;
}

.vault.selected,
.item.selected {
  background: var(--color-selected);
}

.badge {
  font-size: 11px;
}

.new-vault-button {
  margin: 8px;
}

.new-vault {
  margin: 8px 0;
}

.new-vault input {
  margin-bottom: 6px;
}

.account {
  display: flex;
  align-items: center;
  gap: 8px;
}

.account-email {
  flex: 1;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.list {
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--color-border);
  overflow: hidden;
}

.list-header {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 6px 10px 0 10px;
  height: auto;
  min-height: var(--title-bar-height);
}

.list-header input {
  --wails-draggable: no-drag;
}

.list-header button {
  --wails-draggable: no-drag;
  padding: 6px 12px;
}

.list ul {
  flex: 1;
  margin: 8px 0 0;
  padding: 0 6px;
  list-style: none;
  overflow-y: auto;
}

.item {
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
}

.item-title {
  font-weight: 600;
}

.item-subtitle {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.detail {
  padding: 0 28px 28px;
  overflow-y: auto;
}

.detail-error {
  margin-top: 4px;
}

.empty {
  padding: 16px;
}
</style>
