<script setup lang="ts">
import { ref, watch } from "vue";
import type { ItemPasskey } from "../api";
import { deletePasskey, store } from "../store";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{ passkeys: ItemPasskey[]; canDelete: boolean }>();

const confirmingCredentialID = ref("");

watch(
  () => props.passkeys,
  () => (confirmingCredentialID.value = ""),
);

function formatCreatedAt(timestamp: string): string {
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
}

async function confirmDelete(credentialID: string): Promise<void> {
  await deletePasskey(credentialID);
  confirmingCredentialID.value = "";
}
</script>

<template>
  <section class="card passkey-card" data-testid="passkey-section">
    <p class="section-label">Passkeys</p>
    <div v-for="passkey in passkeys" :key="passkey.credentialId" class="passkey" data-testid="passkey-row">
      <span class="passkey-icon" aria-hidden="true"><AppIcon name="passkey" :size="16" /></span>
      <span class="passkey-text">
        <span class="passkey-site" data-testid="passkey-site">{{ passkey.relyingPartyId }}</span>
        <span class="muted passkey-meta">
          <span data-testid="passkey-user">{{ passkey.userName || passkey.userDisplayName || "No user name" }}</span>
          <span v-if="formatCreatedAt(passkey.createdAt)" data-testid="passkey-created"> · Created {{ formatCreatedAt(passkey.createdAt) }}</span>
        </span>
      </span>
      <template v-if="canDelete">
        <span v-if="confirmingCredentialID === passkey.credentialId" class="confirm" role="alertdialog">
          <button type="button" class="ghost" @click="confirmingCredentialID = ''">Cancel</button>
          <button type="button" class="danger-solid" data-testid="confirm-delete-passkey" :disabled="store.isBusy" @click="confirmDelete(passkey.credentialId)">Delete passkey</button>
        </span>
        <button
          v-else
          type="button"
          class="ghost icon-button"
          :aria-label="`Delete passkey for ${passkey.relyingPartyId}`"
          title="Delete passkey"
          data-testid="delete-passkey"
          @click="confirmingCredentialID = passkey.credentialId"
        >
          <AppIcon name="trash" />
        </button>
      </template>
    </div>
    <p class="muted passkey-hint">Hisar signs in with passkeys through the browser extension while Hisar is unlocked.</p>
  </section>
</template>

<style scoped>
.passkey-card {
  margin-top: 14px;
  padding: 14px 10px 10px 16px;
}

.section-label {
  margin: 0 0 6px;
}

.passkey {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 48px;
  border-bottom: 1px solid var(--color-border);
}

.passkey-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 1px solid var(--color-border-strong);
  border-radius: 8px;
  color: var(--color-text-secondary);
}

.passkey-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.passkey-site {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.passkey-meta {
  font-size: var(--font-size-12);
}

.confirm {
  display: flex;
  gap: 6px;
}

.passkey-hint {
  margin: 10px 0 0;
  font-size: var(--font-size-12);
}
</style>
