<script setup lang="ts">
import { computed, ref } from "vue";
import { api, getErrorMessage } from "../api";
import { retryReads, store } from "../store";
import ReadIssueDeletionDialog from "./ReadIssueDeletionDialog.vue";

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
  <section v-if="issues.length || pending || error" class="read-issues" role="alert" data-testid="read-issues">
    <details v-if="issues.length">
      <summary>{{ issues.length }} unreadable vault or item records. Items that open remain available until deleted.</summary>
      <p>Retry before repairing. An unreadable name can hide a vault's name while its items still open. Deleting that vault also deletes its readable items for everyone who has it.</p>
      <button @click="retryReads">Retry reads</button>
      <ul>
        <li v-for="entry in issues" :key="`${entry.issue.vaultId}/${entry.issue.itemId}/${entry.issue.kind}`">
          {{ entry.vault.name }}. {{ entry.issue.kind }}. {{ entry.issue.itemId || entry.issue.vaultId }}
          <template v-if="entry.issue.canRename">
            <input v-model="repairName" aria-label="Repaired vault name" placeholder="New vault name" />
            <button :disabled="!repairName.trim()" @click="rename(entry.issue.vaultId, entry.issue.revision ?? '')">Repair vault name</button>
          </template>
          <button v-if="entry.issue.canDelete" @click="pending = { vault: { ...entry.vault, readIssues: entry.vault.readIssues?.map(issue => ({ ...issue })) }, issue: { ...entry.issue } }">{{ entry.issue.itemId ? 'Delete unreadable data…' : 'Delete vault…' }}</button>
        </li>
      </ul>
    </details>
    <ReadIssueDeletionDialog v-if="pending" :vault="pending.vault" :issue="pending.issue" @cancel="pending = null" @confirm="remove" />
    <p v-if="error">{{ error }}</p>
  </section>
</template>

<style scoped>
.read-issues { padding: 8px 12px; color: var(--color-warning-text); background: var(--color-warning-background); font-size: var(--font-size-12); }
li { margin: 8px 0; overflow-wrap: anywhere; }
button, input { margin: 4px; }
</style>
