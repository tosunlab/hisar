<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { api, getErrorMessage } from "../api";
import { retryReads, store } from "../store";

const issues = computed(() => store.allVault.flatMap((vault) => (vault.readIssues ?? []).map((issue) => ({ vault, issue }))));
const error = ref("");
const pending = ref<(typeof issues.value)[number] | null>(null);
const repairNames = ref<Record<string, string>>({});
watch(issues, (entries) => {
  const current = new Set(entries.filter(({ issue }) => issue.canRename).map(({ issue }) => `${issue.vaultId}/${issue.revision ?? ""}`));
  for (const key of Object.keys(repairNames.value)) {
    if (!current.has(key)) delete repairNames.value[key];
  }
}, { deep: true });
const explanationByKind: Record<string, string> = {
  "vault-name": "Vault name could not be opened. Its readable items remain available.",
  "vault-key": "Vault key could not be opened. Items in this vault cannot be displayed.",
  item: "Item could not be opened. It remains saved.",
};
const summary = computed(() => {
  const itemCount = new Set(issues.value.filter(({ issue }) => issue.kind === "item" && issue.itemId).map(({ issue }) => `${issue.vaultId}/${issue.itemId}`)).size;
  const vaultCount = new Set(issues.value.map(({ issue }) => issue.vaultId)).size;
  const vaults = `${vaultCount} affected ${vaultCount === 1 ? "vault" : "vaults"}`;
  const counts = itemCount > 0 ? `${itemCount} known unreadable ${itemCount === 1 ? "item" : "items"} in ${vaults}.` : `${vaults}.`;
  return issues.value.some(({ issue }) => issue.kind === "vault-key") ? `${counts} Items behind unopened vault keys are not counted.` : counts;
});

// Each action names the revision on screen; the app refuses it once the record changed, and the retry shows the new one.
async function remove(): Promise<void> {
  const entry = pending.value;
  if (!entry) return;
  pending.value = null;
  error.value = "";
  try {
    if (entry.issue.itemId) await api.deleteUnreadableItem(entry.issue.vaultId, entry.issue.itemId, entry.issue.revision ?? "");
    else await api.deleteUnreadableVault(entry.issue.vaultId, entry.issue.revision ?? "");
  } catch (cause) {
    error.value = getErrorMessage(cause);
  }
  await retryReads();
}

async function rename(vaultID: string, revision: string): Promise<void> {
  error.value = "";
  try {
    const key = `${vaultID}/${revision}`;
    await api.repairVaultName(vaultID, revision, repairNames.value[key] ?? "");
    delete repairNames.value[key];
  } catch (cause) {
    error.value = getErrorMessage(cause);
  }
  await retryReads();
}
</script>

<template>
  <section v-if="issues.length" class="read-issues" role="alert" data-testid="read-issues">
    <details>
      <summary>{{ summary }}</summary>
      <div class="read-issues-body" role="region" aria-label="Unreadable records" tabindex="0" data-testid="read-issues-records">
        <p>Retry before repairing. Unreadable data is not an empty vault or proof that an item was deleted.</p>
        <button @click="retryReads">Retry reads</button>
        <ul>
          <li v-for="entry in issues" :key="`${entry.issue.vaultId}/${entry.issue.itemId}/${entry.issue.kind}/${entry.issue.revision}`">
            {{ entry.vault.name }}. {{ explanationByKind[entry.issue.kind] ?? "Saved data could not be opened. It remains saved." }}
            <details><summary>Details</summary>{{ entry.issue.itemId || entry.issue.vaultId }}</details>
            <template v-if="entry.issue.canRename">
              <input v-model="repairNames[`${entry.issue.vaultId}/${entry.issue.revision ?? ''}`]" :aria-label="`New name for ${entry.issue.vaultId}`" placeholder="New vault name" />
              <button :disabled="!repairNames[`${entry.issue.vaultId}/${entry.issue.revision ?? ''}`]?.trim()" @click="rename(entry.issue.vaultId, entry.issue.revision ?? '')">Repair name</button>
            </template>
            <button v-if="entry.issue.canDelete" @click="pending = entry">Delete unreadable data…</button>
          </li>
        </ul>
        <div v-if="pending" role="alertdialog" aria-label="Delete unreadable data">
          <p>Permanently delete {{ pending.issue.itemId ? 'this unreadable item' : 'this vault and every item in it' }} on all devices? This cannot be undone.</p>
          <button @click="pending = null">Cancel</button>
          <button @click="remove">Permanently delete</button>
        </div>
        <p v-if="error">{{ error }}</p>
      </div>
    </details>
  </section>
</template>

<style scoped>
.read-issues { flex: 0 0 auto; padding: 8px 12px; color: var(--color-warning-text); background: var(--color-warning-background); font-size: var(--font-size-12); }
.read-issues-body { max-height: min(25dvh, 160px); overflow-y: auto; overscroll-behavior: contain; }
li { margin: 8px 0; overflow-wrap: anywhere; }
button, input { margin: 4px; }
</style>
