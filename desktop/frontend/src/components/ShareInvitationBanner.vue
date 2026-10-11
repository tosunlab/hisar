<script setup lang="ts">
import { acceptShareInvitation, declineShareInvitation, store } from "../store";
import { formatRole } from "../util";
import AppIcon from "./AppIcon.vue";

function describeInviter(name: string, email: string): string {
  return name && name !== email ? name : email;
}
</script>

<template>
  <div v-if="store.shareInvitations.received.length > 0" class="banner-list" data-testid="share-invitation-banner">
    <div v-for="invitation in store.shareInvitations.received" :key="invitation.id" class="banner" role="status">
      <div class="banner-text">
        <AppIcon name="users" :size="15" />
        <span>
          <strong>{{ describeInviter(invitation.invitedByName, invitation.invitedByEmail) }}</strong> shared a vault with you as
          {{ formatRole(invitation.role).toLowerCase() }}
        </span>
      </div>
      <div class="banner-actions">
        <button class="ghost" :disabled="store.isBusy" :data-testid="`banner-decline-${invitation.id}`" @click="declineShareInvitation(invitation)">Decline</button>
        <button class="primary" :disabled="store.isBusy" :data-testid="`banner-accept-${invitation.id}`" @click="acceptShareInvitation(invitation)">Accept</button>
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
