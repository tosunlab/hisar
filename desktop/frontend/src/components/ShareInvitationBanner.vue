<script setup lang="ts">
import { computed } from "vue";
import type { SentShareInvitation } from "../api";
import { acceptShareInvitation, declineShareInvitation, store } from "../store";
import { describePerson, formatRole, formatTimeAgo } from "../util";
import AppIcon from "./AppIcon.vue";
import ShareRecipientConfirmation from "./ShareRecipientConfirmation.vue";

const allWaiting = computed(() => store.shareInvitations.waiting ?? []);
const allAcceptedSent = computed(() => store.shareInvitations.sent.filter((invitation) => invitation.state === "accepted"));
const hasBanner = computed(() => store.shareInvitations.received.length > 0 || allWaiting.value.length > 0 || allAcceptedSent.value.length > 0);

function findCompletionError(invitation: SentShareInvitation): string {
  const email = invitation.email.toLowerCase();
  return store.shareInvitations.completionError
    .split("\n")
    .filter((line) => email !== "" && line.toLowerCase().includes(email))
    .join(" ");
}
</script>

<template>
  <div v-if="hasBanner" class="banner-list" data-testid="share-invitation-banner">
    <div v-for="invitation in store.shareInvitations.received" :key="invitation.id" class="banner" role="status">
      <div class="banner-text">
        <AppIcon name="users" :size="15" />
        <span>
          <strong>{{ describePerson(invitation.invitedByName, invitation.invitedByEmail) }}</strong> shared a vault with you as
          {{ formatRole(invitation.role).toLowerCase() }}
        </span>
      </div>
      <div class="banner-actions">
        <button class="ghost" :disabled="store.isBusy" :data-testid="`banner-decline-${invitation.id}`" @click="declineShareInvitation(invitation)">Decline</button>
        <button class="primary" :disabled="store.isBusy" :data-testid="`banner-accept-${invitation.id}`" @click="acceptShareInvitation(invitation)">Accept</button>
      </div>
    </div>
    <div v-for="invitation in allAcceptedSent" :key="invitation.id" class="banner" role="status" :data-testid="`banner-accepted-${invitation.id}`">
      <div class="banner-text">
        <AppIcon name="users" :size="15" />
        <span>
          <strong>{{ describePerson(invitation.recipientName, invitation.email) }}</strong> accepted your invitation to
          <strong>{{ invitation.vaultName || "a vault" }}</strong>.
          <template v-if="findCompletionError(invitation)">Hisar couldn't finish sharing: {{ findCompletionError(invitation) }}</template>
          <template v-else-if="invitation.requiresConfirmation">Review this person's identity before giving access.</template>
          <template v-else>Finishing sharing…</template>
        </span>
      </div>
      <ShareRecipientConfirmation :invitation="invitation" />
    </div>
    <div v-for="invitation in allWaiting" :key="invitation.id" class="banner" role="status" :data-testid="`banner-waiting-${invitation.id}`">
      <div class="banner-text">
        <AppIcon name="clock" :size="15" />
        <span>
          Waiting for <strong>{{ describePerson(invitation.invitedByName, invitation.invitedByEmail) }}</strong>'s Hisar to finish sharing
          <span class="since">· accepted {{ formatTimeAgo(invitation.acceptedAt) }}</span>
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.banner-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0 0 14px;
}

.banner {
  padding: 10px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: 12px;
  line-height: 1.4;
}

.banner-text {
  display: flex;
  gap: 8px;
  color: var(--color-text-secondary);
}

.banner-text strong {
  color: var(--color-text);
}

.since {
  color: var(--color-text-muted);
  white-space: nowrap;
}

.banner-text svg {
  flex: none;
  margin-top: 1px;
  color: var(--color-text-muted);
}

.banner-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  margin-top: 8px;
}

.banner-actions button {
  padding: 4px 10px;
  font-size: 12px;
}
</style>
