<script setup lang="ts">
import { ref } from "vue";
import { clearError, replaceSecretKey, store } from "../store";
import AccountDialog from "./AccountDialog.vue";
import CredentialField from "./CredentialField.vue";

const emit = defineEmits<{ close: [] }>();
const password = ref("");

async function submitReplacement(): Promise<void> {
  if (!password.value || store.isBusy) return;
  const result = await replaceSecretKey(password.value);
  password.value = "";
  if (result) emit("close");
}

function close(): void {
  clearError();
  emit("close");
}
</script>

<template>
  <AccountDialog
    title="Replace Secret Key"
    subtitle="Your other devices will be signed out. Sign in on them with your master password and the new Secret Key from the new Emergency Kit."
    icon="key"
    :is-closable="!store.isBusy"
    data-testid="replace-secret-key-dialog"
    @close="close"
  >
    <form novalidate @submit.prevent="submitReplacement">
      <ul class="consequences warning" data-testid="replace-secret-key-consequences">
        <li>Your old Emergency Kit stops working.</li>
        <li>Your master password stays the same.</li>
      </ul>
      <label>
        <span>Master password</span>
        <CredentialField><input v-model="password" name="replaceSecretKeyPassword" type="password" autocomplete="current-password" autofocus /></CredentialField>
      </label>
      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      <div class="row actions">
        <button type="button" class="ghost" :disabled="store.isBusy" @click="close">Cancel</button>
        <button class="primary" type="submit" :disabled="!password || store.isBusy" data-testid="confirm-replace-secret-key">
          {{ store.isBusy ? "Replacing…" : "Replace Secret Key" }}
        </button>
      </div>
    </form>
  </AccountDialog>
</template>
