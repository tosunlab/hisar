<script setup lang="ts">
import { nextTick, onMounted, ref } from "vue";
import { api, getErrorMessage, type ItemDetail, type VaultSummary } from "../api";
import { selectVault, showItem, startCreatingSSHKey, store } from "../store";
import { sshKeyCategory } from "../util";
import AccountDialog from "./AccountDialog.vue";

const allKey = ref<{ vault: VaultSummary; item: ItemDetail }[] | null>(null);
const errorMessage = ref("");

async function loadKeys(): Promise<void> {
  errorMessage.value = "";
  try {
    allKey.value = (await Promise.all(store.allVault.map(async (vault) => ({ vault, items: await api.listAllItem(vault.id) }))))
      .flatMap(({ vault, items }) => items.filter((item) => item.category === sshKeyCategory && !!item.publicKey && !item.readIssue).map((item) => ({ vault, item })));
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  }
}

async function chooseKey(vaultID: string, itemID: string): Promise<void> {
  await showItem({ vaultID, itemID, isEditing: false });
  if (store.selectedItem?.id !== itemID || store.selectedVaultID !== vaultID) return;
  store.commitSigningItemID = itemID;
  store.isCommitSigningSetupOpen = false;
  await nextTick();
  document.querySelector<HTMLButtonElement>('[data-testid="set-up-commit-signing"]')?.focus();
}

async function newKey(): Promise<void> {
  const vault = store.allVault.find((vault) => vault.type === "personal") ?? store.allVault.find((vault) => vault.role !== "reader");
  if (!vault) return;
  await selectVault(vault.id);
  await startCreatingSSHKey();
  if (store.editorMode === "create-ssh-key") store.isCommitSigningSetupOpen = false;
}

onMounted(loadKeys);
</script>

<template>
  <AccountDialog title="Set up commit signing" subtitle="Choose an SSH key to open its setup commands. Hisar does not change your Git configuration." icon="key" @close="store.isCommitSigningSetupOpen = false">
      <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }} <button @click="loadKeys">Try again</button></p>
      <p v-else-if="allKey === null" role="status">Loading SSH keys…</p>
      <template v-else-if="allKey.length === 0">
        <p>You need an SSH key to sign commits.</p>
        <button class="primary" data-testid="signing-new-ssh-key" @click="newKey">New SSH key…</button>
      </template>
      <ul v-else>
        <li v-for="key in allKey" :key="`${key.vault.id}/${key.item.id}`">
          <button data-testid="signing-choose-key" @click="chooseKey(key.vault.id, key.item.id)">{{ key.item.title }} ({{ key.vault.name }})</button>
        </li>
      </ul>
      <button data-testid="close-signing-chooser" @click="store.isCommitSigningSetupOpen = false">Cancel</button>
  </AccountDialog>
</template>

<style scoped>
button { max-width: 100%; overflow-wrap: anywhere; }
ul { list-style: none; padding: 0; }
li { margin: 8px 0; }
</style>
