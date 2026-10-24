<script setup lang="ts">
import { computed, ref } from "vue";
import { api, getErrorMessage } from "../api";
import { retryReads, store } from "../store";

const issues = computed(() => store.allVault.flatMap((vault) => (vault.readIssues ?? []).map((issue) => ({ vault, issue }))));
const error = ref("");
const pending = ref<(typeof issues.value)[number] | null>(null);
const repairName = ref("");

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
    await api.repairVaultName(vaultID, revision, repairName.value);
    repairName.value = "";
  } catch (cause) {
    error.value = getErrorMessage(cause);
  }
  await retryReads();
}
</script>

<template>
  <section v-if="issues.length" class="read-issues" role="alert" data-testid="read-issues">
    <details>
      <summary>{{ issues.length }} unreadable vault or item records. Healthy items remain available.</summary>
      <p>Retry before repairing. An unreadable record is not an empty vault or proof that an item was deleted.</p>
      <button @click="retryReads">Retry reads</button>
      <ul>
        <li v-for="entry in issues" :key="`${entry.issue.vaultId}/${entry.issue.itemId}/${entry.issue.kind}`">
          {{ entry.vault.name }}. {{ entry.issue.kind }}. {{ entry.issue.itemId || entry.issue.vaultId }}
          <template v-if="entry.issue.canRename">
            <input v-model="repairName" aria-label="Repaired vault name" placeholder="New vault name" />
            <button :disabled="!repairName.trim()" @click="rename(entry.issue.vaultId, entry.issue.revision ?? '')">Repair name</button>
          </template>
          <button v-if="entry.issue.canDelete" @click="pending = entry">Delete unreadable data…</button>
        </li>
      </ul>
    </details>
    <div v-if="pending" role="alertdialog" aria-label="Delete unreadable data">
      <p>Permanently delete {{ pending.issue.itemId ? 'this unreadable item' : 'this vault and every item in it' }} on all devices? This cannot be undone.</p>
      <button @click="pending = null">Cancel</button>
      <button @click="remove">Permanently delete</button>
    </div>
    <p v-if="error">{{ error }}</p>
  </section>
</template>

<style scoped>
.read-issues { padding: 8px 12px; color: var(--color-warning-text); background: var(--color-warning-background); font-size: 12px; }
li { margin: 8px 0; overflow-wrap: anywhere; }
button, input { margin: 4px; }
</style>
