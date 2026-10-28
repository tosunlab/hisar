<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { AccountDeletionPreview, DeletionVault } from "../api";
import { clearError, deleteAccount, loadAccountDeletionPreview, store } from "../store";
import { getThisDeviceName } from "../util";
import AccountDialog from "./AccountDialog.vue";
import CredentialField from "./CredentialField.vue";

const emit = defineEmits<{ close: [] }>();

const preview = ref<AccountDeletionPreview | null>(null);
const password = ref("");
const typedEmail = ref("");

const isEmailMatching = computed(() => preview.value !== null && typedEmail.value.trim().toLowerCase() === preview.value.email.toLowerCase());
const isSubmittable = computed(() => !!preview.value?.planVersion && isEmailMatching.value && password.value !== "" && !store.isBusy);
const allSharedOwnedVault = computed(() => preview.value?.ownedVaults.filter((vault) => vault.otherMemberCount > 0) ?? []);
const allPrivateOwnedVault = computed(() => preview.value?.ownedVaults.filter((vault) => vault.otherMemberCount === 0) ?? []);

function countPeople(count: number): string {
  return count === 1 ? "1 person loses access" : `${count} people lose access`;
}

function countThing(count: number, singular: string, plural: string): string {
  return `${count} ${count === 1 ? singular : plural}`;
}

function describeMembership(vault: DeletionVault): string {
  return vault.role === "owner" ? "you co-own it; it stays with its other owners" : `you are ${vault.role === "editor" ? "an editor" : "a reader"}; your access is removed`;
}

function describePrivateVaults(allVault: DeletionVault[]): string {
  return allVault.map((vault) => vault.name || vault.id).join(", ");
}

onMounted(async () => {
  preview.value = (await loadAccountDeletionPreview()) ?? null;
});

async function submitDeletion(): Promise<void> {
  if (!isSubmittable.value) {
    return;
  }
  const isDeleted = await deleteAccount(password.value, typedEmail.value, preview.value!.planVersion);
  password.value = "";
  if (isDeleted) {
    emit("close");
  } else {
    typedEmail.value = "";
    const failure = store.errorMessage;
    preview.value = (await loadAccountDeletionPreview()) ?? null;
    if (preview.value) {
      store.errorMessage = failure.includes("deletion plan changed")
        ? `${failure} Review the current consequences and enter your email and password again to confirm.`
        : failure;
    }
  }
}

function close(): void {
  clearError();
  emit("close");
}
</script>

<template>
  <AccountDialog
    title="Delete account"
    subtitle="This deletes your Hisar account from the server. It can't be undone."
    icon="trash"
    is-danger
    :is-closable="!store.isBusy"
    data-testid="delete-account-dialog"
    @close="close"
  >
    <p v-if="!preview && !store.errorMessage" class="muted loading">Checking what your account holds…</p>
    <form v-if="preview" novalidate @submit.prevent="submitDeletion">
      <p v-if="!preview.planVersion" class="error" role="alert">This server cannot confirm the reviewed deletion plan. Update the server before deleting your account.</p>
      <p class="export-advice" data-testid="delete-account-export">Export your items first in Settings → Import &amp; Export. Nobody can restore them after the account is deleted.</p>
      <ul class="consequences warning" data-testid="delete-account-consequences">
        <li v-if="allPrivateOwnedVault.length > 0">
          Deleted with their items: {{ describePrivateVaults(allPrivateOwnedVault) }}.
        </li>
        <li v-for="vault in allSharedOwnedVault" :key="vault.id" data-testid="delete-account-owned-shared-vault">
          <strong>{{ vault.name || vault.id }}</strong> is deleted with its items: {{ vault.type === "organization" ? "its organization is deleted" : "you are its only owner" }}, so {{ countPeople(vault.otherMemberCount) }}.
        </li>
        <li v-for="vault in preview.memberVaults" :key="vault.id" data-testid="delete-account-member-vault">
          <strong>{{ vault.name || vault.id }}</strong>: {{ describeMembership(vault) }}.
        </li>
        <li v-for="organization in preview.organizations" :key="organization.id" data-testid="delete-account-organization">
          <template v-if="organization.isDeleted"><strong>{{ organization.name }}</strong> is deleted with its teams, vaults, and items.</template>
          <template v-else-if="organization.newAdminId || organization.newAdminName">
            <strong>{{ organization.newAdminName || organization.newAdminId }}</strong> becomes an admin of <strong>{{ organization.name }}</strong>.
          </template>
          <template v-else>You leave <strong>{{ organization.name }}</strong>.</template>
        </li>
        <li v-if="preview.shareLinkCount > 0">{{ countThing(preview.shareLinkCount, "share link stops", "share links stop") }} working.</li>
        <li v-if="preview.sentInvitationCount + preview.receivedInvitationCount > 0">
          {{ countThing(preview.sentInvitationCount + preview.receivedInvitationCount, "invitation is", "invitations are") }} removed.
        </li>
        <li>{{ countThing(preview.deviceCount, "device is", "devices are") }} signed out, including {{ getThisDeviceName() }}.</li>
      </ul>
      <label>
        <span>Master password</span>
        <CredentialField><input v-model="password" name="deleteAccountPassword" type="password" autocomplete="current-password" autofocus /></CredentialField>
      </label>
      <label>
        <span>Type {{ preview.email }} to confirm</span>
        <input v-model="typedEmail" name="deleteAccountEmail" type="email" autocomplete="off" spellcheck="false" />
      </label>
      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      <div class="row actions">
        <button type="button" class="ghost" :disabled="store.isBusy" @click="close">Cancel</button>
        <button class="danger-solid" type="submit" :disabled="!isSubmittable" data-testid="confirm-delete-account">
          {{ store.isBusy ? "Deleting…" : "Delete account" }}
        </button>
      </div>
    </form>
    <template v-else-if="store.errorMessage">
      <p class="error" role="alert">{{ store.errorMessage }}</p>
      <div class="row actions">
        <button type="button" @click="close">Close</button>
      </div>
    </template>
  </AccountDialog>
</template>

<style scoped>
.export-advice {
  margin: 0 0 10px;
  font-size: var(--font-size-13);
}

.loading {
  margin: 0 0 12px;
  font-size: var(--font-size-12);
}
</style>
