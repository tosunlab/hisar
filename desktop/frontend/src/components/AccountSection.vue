<script setup lang="ts">
import { ref } from "vue";
import { signOut, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import DeleteAccountDialog from "./DeleteAccountDialog.vue";

const isRowVisible = useSettingsRowVisibility();
const isConfirmingSignOut = ref(false);
const isDeletingAccount = ref(false);
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
          <button v-if="!isConfirmingSignOut" class="danger" @click="isConfirmingSignOut = true">Sign out</button>
          <template v-else>
            <button class="ghost" @click="isConfirmingSignOut = false">Cancel</button>
            <button class="danger-solid" data-testid="settings-confirm-sign-out" @click="signOut">Sign out</button>
          </template>
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
