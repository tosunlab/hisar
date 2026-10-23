<script setup lang="ts">
import { ref } from "vue";
import type { SentShareInvitation } from "../api";
import { confirmShareRecipient, store } from "../store";
import { formatRole } from "../util";
import AccountDialog from "./AccountDialog.vue";

const props = defineProps<{ invitation: SentShareInvitation }>();
const reviewed = ref<SentShareInvitation | null>(null);
async function confirm(): Promise<void> {
  if (reviewed.value && await confirmShareRecipient(reviewed.value)) reviewed.value = null;
}
</script>

<template>
  <button v-if="invitation.requiresConfirmation" class="primary" :disabled="store.isBusy" :data-testid="`review-share-${invitation.id}`" @click="reviewed = { ...props.invitation }">Review recipient</button>
  <AccountDialog v-if="reviewed" title="Give this person access?" subtitle="Compare the security code with the recipient over a channel you trust before confirming." icon="users" :is-closable="!store.isBusy" data-testid="share-recipient-confirmation" @close="reviewed = null">
    <p>{{ reviewed.recipientName || reviewed.email }} · {{ reviewed.email }}</p>
    <p>Vault {{ reviewed.vaultName }} · {{ formatRole(reviewed.role) }}</p>
    <p>Account {{ reviewed.recipientId }}</p>
    <p class="mono">Security code {{ reviewed.securityCode }}</p>
    <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
    <div class="row actions">
      <button class="ghost" :disabled="store.isBusy" @click="reviewed = null">Cancel</button>
      <button class="primary" :disabled="store.isBusy" data-testid="confirm-share-recipient" @click="confirm">Give access</button>
    </div>
  </AccountDialog>
</template>
