<script setup lang="ts">
import { nextTick, ref } from "vue";
import type { ItemPasskey } from "../api";
import { deletePasskey, store } from "../store";
import AppIcon from "./AppIcon.vue";
import AccountDialog from "./AccountDialog.vue";

const props = defineProps<{ passkeys: ItemPasskey[]; canDelete: boolean; vaultName: string; vaultType: string }>();
const selectedPasskey = ref<ItemPasskey | null>(null);
const errorMessage = ref("");
const cancelButton = ref<HTMLButtonElement | null>(null);
let deleteTrigger: HTMLElement | null = null;

async function requestDelete(passkey: ItemPasskey, event: Event): Promise<void> {
  deleteTrigger = event.currentTarget as HTMLElement;
  errorMessage.value = "";
  selectedPasskey.value = passkey;
  await nextTick();
  cancelButton.value?.focus();
}

async function cancelDelete(): Promise<void> {
  selectedPasskey.value = null;
  await nextTick();
  deleteTrigger?.focus();
}

function description(passkey: ItemPasskey): string {
  const account = passkey.userName || passkey.userDisplayName;
  const consequence = props.vaultType === "personal"
    ? "It is removed from all your devices. Make sure you can sign in another way first."
    : `It is removed from ${props.vaultName || "this vault"} for everyone with access. Make sure they can sign in another way first.`;
  return `You can no longer sign in to ${passkey.relyingPartyId} as ${account} with it. ${consequence}`;
}

function formatCreatedAt(timestamp: string): string {
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
}

async function confirmDelete(): Promise<void> {
  if (!selectedPasskey.value) return;
  if (!(await deletePasskey(selectedPasskey.value.credentialId))) {
    errorMessage.value = store.errorMessage || "The passkey could not be deleted.";
    return;
  }
  selectedPasskey.value = null;
  await nextTick();
  (document.querySelector<HTMLElement>('[data-testid="passkey-section"]') ?? document.querySelector<HTMLElement>('[data-testid="item-heading"]'))?.focus();
}
</script>

<template>
  <section class="card passkey-card" tabindex="-1" aria-label="Passkeys" data-testid="passkey-section">
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
        <button
          type="button"
          class="ghost icon-button"
          :aria-label="`Delete passkey for ${passkey.relyingPartyId}`"
          title="Delete passkey"
          data-testid="delete-passkey"
          @click="requestDelete(passkey, $event)"
        >
          <AppIcon name="trash" />
        </button>
      </template>
    </div>
    <p class="muted passkey-hint">Hisar signs in with passkeys through the browser extension while Hisar is unlocked.</p>
  </section>
  <AccountDialog v-if="selectedPasskey" :title="`Delete the passkey for ${selectedPasskey.relyingPartyId}?`"
    :subtitle="description(selectedPasskey)" icon="passkey" is-danger :is-closable="!store.isBusy" @close="cancelDelete">
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <div class="row actions">
      <button ref="cancelButton" type="button" class="ghost" :disabled="store.isBusy" @click="cancelDelete">Cancel</button>
      <button type="button" class="danger-solid" data-testid="confirm-delete-passkey" :disabled="store.isBusy" @click="confirmDelete">Delete passkey</button>
    </div>
  </AccountDialog>
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


.passkey-hint {
  margin: 10px 0 0;
  font-size: var(--font-size-12);
}
</style>
