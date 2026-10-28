<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { openFromSettings, signOut, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { getSignOutCopy } from "../util";
import DeleteAccountDialog from "./DeleteAccountDialog.vue";

const emit = defineEmits<{ saveEmergencyKit: [] }>();
const signOutCopy = computed(() => getSignOutCopy(false, store.status.emergencyKit));
const isRowVisible = useSettingsRowVisibility();
const isConfirmingSignOut = ref(false);
const isDeletingAccount = ref(false);
const cancelSignOutButton = ref<HTMLButtonElement | null>(null);
async function confirmSignOut(): Promise<void> {
  isConfirmingSignOut.value = true;
  await nextTick();
  cancelSignOutButton.value?.focus();
}
</script>

<template>
  <section data-testid="account-section">
    <div v-show="isRowVisible('account')" class="settings-block">
      <p class="section-label">Signed in as</p>
      <div class="settings-group">
        <div class="setting">
          <div class="setting-text">
            <strong>{{ store.status.name || store.status.email }}</strong>
            <p class="muted">{{ store.status.email }} · {{ store.status.server }}</p>
          </div>
          <button v-if="!isConfirmingSignOut" class="danger" @click="confirmSignOut">Sign out</button>
        </div>
        <div v-if="isConfirmingSignOut" class="card confirm" data-testid="settings-sign-out-confirmation">
          <h3>{{ signOutCopy.title }}</h3>
          <p>{{ signOutCopy.message }}</p>
          <div class="row">
            <button ref="cancelSignOutButton" class="ghost" @click="isConfirmingSignOut = false">Cancel</button>
            <button data-testid="sign-out-save-emergency-kit" @click="isConfirmingSignOut = false; store.status.emergencyKit ? openFromSettings('recovery') : emit('saveEmergencyKit')">Save Emergency Kit…</button>
            <button class="danger-solid" data-testid="settings-confirm-sign-out" @click="signOut">Sign out</button>
          </div>
        </div>
      </div>
    </div>

    <div v-show="isRowVisible('delete-account')" class="settings-block">
      <p class="section-label">Delete account</p>
      <div class="settings-group">
        <div class="setting">
          <div class="setting-text">
            <strong>Delete your Hisar account</strong>
            <p class="muted">
              Deletes your account, your vaults that have no other owner and their items, your share links and invitations, and signs out
              every device. Vaults with another owner stay with them.
            </p>
          </div>
          <button class="danger" data-testid="delete-account" @click="isDeletingAccount = true">Delete account…</button>
        </div>
      </div>
    </div>

    <DeleteAccountDialog v-if="isDeletingAccount" @close="isDeletingAccount = false" />
  </section>
</template>

<style scoped>
.confirm {
  margin: 12px;
  padding: 14px 16px;
}

.confirm h3,
.confirm p {
  margin: 0 0 10px;
}

.confirm .row {
  flex-wrap: wrap;
}
</style>
