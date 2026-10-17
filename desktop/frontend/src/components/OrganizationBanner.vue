<script setup lang="ts">
import { computed } from "vue";
import { acceptOrganizationInvitation, declineOrganizationInvitation, store } from "../store";
import { describePerson, formatRoleWithArticle } from "../util";
import AppIcon from "./AppIcon.vue";

interface WaitingOrganization {
  organizationID: string;
  organizationName: string;
  vaultCount: number;
}

// allWaitingOrganization counts per organization the vaults still waiting for a key; a vault already in the list arrived.
const allWaitingOrganization = computed(() => {
  const waitingByID = new Map<string, WaitingOrganization>();
  for (const waiting of store.waitingVaultKeys.waiting) {
    if (store.allVault.some((vault) => vault.id === waiting.vaultId)) {
      continue;
    }
    const organizationName =
      waiting.organizationName || store.organizations.organizations.find((organization) => organization.id === waiting.organizationId)?.name || "an organization";
    const waitingOrganization = waitingByID.get(waiting.organizationId) ?? { organizationID: waiting.organizationId, organizationName, vaultCount: 0 };
    waitingOrganization.vaultCount += 1;
    waitingByID.set(waiting.organizationId, waitingOrganization);
  }
  return [...waitingByID.values()];
});
const hasBanner = computed(
  () =>
    store.organizations.invitations.length > 0 ||
    allWaitingOrganization.value.length > 0 ||
    store.waitingVaultKeys.deliveryError !== "" ||
    store.waitingVaultKeys.rotationError !== "",
);
</script>

<template>
  <div v-if="hasBanner" class="banner-list" data-testid="organization-banner">
    <div
      v-for="invitation in store.organizations.invitations"
      :key="invitation.id"
      class="banner"
      role="status"
      :data-testid="`organization-banner-invitation-${invitation.id}`"
    >
      <div class="banner-text">
        <AppIcon name="building" :size="15" />
        <span>
          <strong>{{ describePerson(invitation.invitedByName, invitation.invitedByEmail) }}</strong> invited you to
          <strong>{{ invitation.organizationName }}</strong> as {{ formatRoleWithArticle(invitation.role) }}
        </span>
      </div>
      <div class="banner-actions">
        <button class="ghost" :disabled="store.isBusy" :data-testid="`organization-banner-decline-${invitation.id}`" @click="declineOrganizationInvitation(invitation)">
          Decline
        </button>
        <button class="primary" :disabled="store.isBusy" :data-testid="`organization-banner-accept-${invitation.id}`" @click="acceptOrganizationInvitation(invitation)">
          Accept
        </button>
      </div>
    </div>
    <div
      v-for="waiting in allWaitingOrganization"
      :key="waiting.organizationID"
      class="banner"
      role="status"
      :data-testid="`organization-banner-waiting-${waiting.organizationID}`"
    >
      <div class="banner-text">
        <AppIcon name="clock" :size="15" />
        <span v-if="waiting.vaultCount === 1">Waiting for an owner of a vault in {{ waiting.organizationName }} to give you its key</span>
        <span v-else>Waiting for the owners of {{ waiting.vaultCount }} vaults in {{ waiting.organizationName }} to give you their keys</span>
      </div>
    </div>
    <div v-if="store.waitingVaultKeys.deliveryError" class="banner" role="status" data-testid="organization-banner-delivery-error">
      <div class="banner-text">
        <AppIcon name="warning" :size="15" class="warning-icon" />
        <span>Hisar couldn't give every vault key: {{ store.waitingVaultKeys.deliveryError }}</span>
      </div>
    </div>
    <div v-if="store.waitingVaultKeys.rotationError" class="banner" role="status" data-testid="organization-banner-rotation-error">
      <div class="banner-text">
        <AppIcon name="warning" :size="15" class="warning-icon" />
        <span>Hisar couldn't re-encrypt every vault that needs a new key: {{ store.waitingVaultKeys.rotationError }}</span>
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
  overflow-wrap: anywhere;
}

.banner-text strong {
  color: var(--color-text);
}

.banner-text svg {
  flex: none;
  margin-top: 1px;
  color: var(--color-text-muted);
}

.banner-text .warning-icon {
  color: var(--color-warning);
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
