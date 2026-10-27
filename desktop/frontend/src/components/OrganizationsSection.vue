<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { acceptOrganizationInvitation, createOrganization, declineOrganizationInvitation, loadOrganizations, openOrganization, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { describePerson, formatCount, formatRole, formatRoleWithArticle } from "../util";
import AppIcon from "./AppIcon.vue";
import MemberAvatar from "./MemberAvatar.vue";
import OrganizationDetail from "./OrganizationDetail.vue";

const isRowVisible = useSettingsRowVisibility();
const isLoading = ref(false);
const isCreating = ref(false);
const newOrganizationName = ref("");

const selectedOrganization = computed(() => (store.selectedOrganizationID ? store.organizationByID[store.selectedOrganizationID] : undefined));
const selectedOrganizationName = computed(
  () => store.organizations.organizations.find((organization) => organization.id === store.selectedOrganizationID)?.name ?? "",
);

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

async function submitCreate(): Promise<void> {
  if (await createOrganization(newOrganizationName.value.trim())) {
    newOrganizationName.value = "";
    isCreating.value = false;
  }
}

onMounted(async () => {
  isLoading.value = true;
  await loadOrganizations();
  isLoading.value = false;
});
</script>

<template>
  <section data-testid="organizations-section">
    <template v-if="store.selectedOrganizationID">
      <OrganizationDetail v-if="selectedOrganization" :key="selectedOrganization.id" :organization="selectedOrganization" />
      <div v-else class="settings-block">
        <button class="ghost back-button" data-testid="organization-back" @click="store.selectedOrganizationID = ''">
          <AppIcon name="chevronRight" :size="14" class="back-chevron" />Organizations
        </button>
        <p class="muted empty-note">{{ store.isBusy ? `Loading ${selectedOrganizationName}…` : "" }}</p>
      </div>
    </template>

    <template v-else>
      <p v-if="store.organizationsErrorMessage" class="error" role="alert" data-testid="organizations-error">{{ store.organizationsErrorMessage }}</p>

      <div
        v-if="store.organizations.invitations.length > 0"
        v-show="isRowVisible('organization-invitations')"
        class="settings-block"
        data-testid="organization-invitations"
      >
        <p class="section-label">Invitations for you</p>
        <div class="settings-group">
          <div
            v-for="invitation in store.organizations.invitations"
            :key="invitation.id"
            class="setting"
            :data-testid="`organization-invitation-${invitation.id}`"
          >
            <MemberAvatar :name="describePerson(invitation.invitedByName, invitation.invitedByEmail)" />
            <div class="setting-text">
              <strong class="invitation-text">
                {{ describePerson(invitation.invitedByName, invitation.invitedByEmail) }} invited you to {{ invitation.organizationName }} as
                {{ formatRoleWithArticle(invitation.role) }}
              </strong>
              <p class="muted">{{ invitation.invitedByEmail }}</p>
            </div>
            <button class="ghost" :disabled="store.isBusy" :data-testid="`decline-organization-invitation-${invitation.id}`" @click="declineOrganizationInvitation(invitation)">
              Decline
            </button>
            <button class="primary" :disabled="store.isBusy" :data-testid="`accept-organization-invitation-${invitation.id}`" @click="acceptOrganizationInvitation(invitation)">
              Accept
            </button>
          </div>
        </div>
      </div>

      <div v-if="allWaitingOrganization.length > 0" v-show="isRowVisible('organizations')" class="settings-block" data-testid="waiting-vault-keys">
        <p class="section-label">Vault keys on their way</p>
        <div class="settings-group">
          <div v-for="waiting in allWaitingOrganization" :key="waiting.organizationID" class="setting" :data-testid="`organization-waiting-${waiting.organizationID}`">
            <span class="settings-row-icon"><AppIcon name="clock" :size="14" /></span>
            <span v-if="waiting.vaultCount === 1" class="setting-text">Waiting for an owner of a vault in {{ waiting.organizationName }} to give you its key</span>
            <span v-else class="setting-text">Waiting for the owners of {{ waiting.vaultCount }} vaults in {{ waiting.organizationName }} to give you their keys</span>
          </div>
        </div>
      </div>

      <div v-show="isRowVisible('organizations')" class="settings-block">
        <p class="section-label">Your organizations</p>
        <p v-if="store.organizations.isCached" class="muted" role="status">Saved organization names. Connect to load current membership and invitations.</p>
        <p v-if="store.organizations.organizations.length === 0" class="muted empty-note" data-testid="organizations-empty">
          {{ isLoading ? "Loading organizations…" : "You are not in an organization yet. Create one, or ask an admin of one to invite you." }}
        </p>
        <div v-else class="settings-group">
          <button
            v-for="organization in store.organizations.organizations"
            :key="organization.id"
            type="button"
            class="setting organization-row"
            :data-testid="`organization-${organization.id}`"
            :disabled="store.organizations.isCached"
            @click="openOrganization(organization.id)"
          >
            <span class="settings-row-icon"><AppIcon name="building" :size="14" /></span>
            <span class="setting-text">
              <strong class="title-line"><span class="title-text">{{ organization.name }}</span></strong>
              <span v-if="!store.organizations.isCached" class="muted organization-summary">{{ formatRole(organization.role) }} · {{ formatCount(organization.memberCount, "member") }}</span>
            </span>
            <AppIcon name="chevronRight" :size="14" class="row-chevron" />
          </button>
        </div>
      </div>

      <div v-show="isRowVisible('create-organization')" class="settings-block">
        <div v-if="isCreating" class="settings-group">
          <p class="muted create-explanation">You become its first admin. Organization and team names are visible to the server.</p>
          <form class="settings-group-form" data-testid="create-organization-form" @submit.prevent="submitCreate">
            <input
              v-model="newOrganizationName"
              name="organizationName"
              aria-label="Organization name"
              placeholder="Organization name"
              maxlength="100"
              autocomplete="off"
              autofocus
            />
            <button type="button" class="ghost" @click="isCreating = false">Cancel</button>
            <button class="primary" type="submit" data-testid="create-organization" :disabled="store.isBusy || !newOrganizationName.trim()">Create</button>
          </form>
        </div>
        <button v-else data-testid="start-create-organization" @click="isCreating = true"><AppIcon name="plus" :size="14" />Create organization…</button>
      </div>
    </template>
  </section>
</template>

<style scoped>
.organization-row {
  width: 100%;
  justify-content: flex-start;
  border: none;
  border-bottom: 1px solid var(--color-border);
  border-radius: 0;
  background: transparent;
  font-weight: 400;
  text-align: left;
}

.invitation-text {
  display: block;
  line-height: 1.35;
}

.organization-row:hover:not(:disabled) {
  border-color: var(--color-border);
  background: var(--color-hover);
}

.organization-row .setting-text {
  display: flex;
  flex-direction: column;
}

.organization-summary {
  margin-top: 2px;
  font-size: var(--font-size-12);
}

.row-chevron {
  color: var(--color-text-muted);
}

.create-explanation {
  margin: 0;
  padding: 10px 14px 8px;
  font-size: var(--font-size-12);
}

.settings-group-form input {
  flex: 1;
  min-width: 0;
}

.back-button {
  margin: 0 0 12px -8px;
  padding: 4px 8px;
}

.back-chevron {
  transform: rotate(180deg);
}
</style>
