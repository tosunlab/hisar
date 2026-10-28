<script setup lang="ts">
import { computed, ref } from "vue";
import type { SentShareInvitation } from "../api";
import { cancelShareInvitation, confirmShareRecipient, showNotice, store } from "../store";
import { formatRole } from "../util";
import AccountDialog from "./AccountDialog.vue";

const props = defineProps<{ invitation: SentShareInvitation }>();
const reviewed = ref<SentShareInvitation | null>(null);
const errorMessage = ref("");
const isWorking = ref(false);
const isBusy = computed(() => store.isBusy || isWorking.value);

function review(): void {
  errorMessage.value = "";
  reviewed.value = { ...props.invitation };
}

async function confirm(): Promise<void> {
  if (!reviewed.value || isWorking.value) return;
  isWorking.value = true;
  try {
    const snapshot = reviewed.value;
    if (await confirmShareRecipient(snapshot)) reviewed.value = null;
    else errorMessage.value = store.sharingErrors[snapshot.id] || store.errorMessage;
  } finally {
    isWorking.value = false;
  }
}

async function refuse(): Promise<void> {
  const snapshot = reviewed.value;
  if (!snapshot || isWorking.value) return;
  isWorking.value = true;
  try {
    if (await cancelShareInvitation(snapshot)) {
      showNotice(`Cancelled the invitation to ${snapshot.email}.`);
      reviewed.value = null;
    } else {
      errorMessage.value = store.sharingErrors[snapshot.id] || store.errorMessage;
    }
  } finally {
    isWorking.value = false;
  }
}
</script>

<template>
  <button v-if="invitation.requiresConfirmation" class="primary" :disabled="store.isBusy" :data-testid="`review-share-${invitation.id}`" @click="review">Review recipient</button>
  <AccountDialog v-if="reviewed" title="Give this person access?" subtitle="To compare codes, ask this person for the security code Hisar shows for their own account in Settings → People, the row marked you. It must match this code." icon="users" :is-closable="!isBusy" data-testid="share-recipient-confirmation" @close="reviewed = null">
    <p>{{ reviewed.recipientName || reviewed.email }} · {{ reviewed.email }}</p>
    <p>Vault {{ reviewed.vaultName }} · {{ formatRole(reviewed.role) }}</p>
    <p>Account {{ reviewed.recipientId }}</p>
    <div class="review-security-code">
      <p>Security code</p>
      <p class="security-code monospace" data-testid="review-security-code">{{ reviewed.securityCode }}</p>
    </div>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <div class="row actions">
      <button class="ghost" :disabled="isBusy" @click="reviewed = null">Cancel</button>
      <button class="danger" :disabled="isBusy" data-testid="share-recipient-mismatch" @click="refuse">Codes don't match</button>
      <button class="primary" :disabled="isBusy" data-testid="confirm-share-recipient" @click="confirm">Give access</button>
    </div>
  </AccountDialog>
</template>

<style scoped>
.security-code {
  font-size: var(--font-size-17);
  font-weight: 600;
  letter-spacing: 0.12em;
  color: var(--color-text);
  user-select: text;
  -webkit-user-select: text;
}
</style>
