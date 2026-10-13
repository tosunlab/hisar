<script setup lang="ts">
import { computed, ref } from "vue";
import type { CredentialChangeResult } from "../api";
import { clearError, copyText, finishSecretKeyReplacement, replaceSecretKey, saveNewAccountEmergencyKit, store } from "../store";
import { describeSignedOutDevices } from "../util";
import AccountDialog from "./AccountDialog.vue";
import AppIcon from "./AppIcon.vue";

const emit = defineEmits<{ close: [] }>();

const password = ref("");
const result = ref<CredentialChangeResult | null>(null);
const hasSavedEmergencyKit = ref(false);
const hasConfirmedSaved = ref(false);

const allKeyPart = computed(() => result.value?.secretKey.split("-") ?? []);
const isDoneEnabled = computed(() => hasSavedEmergencyKit.value || hasConfirmedSaved.value);

async function submitReplacement(): Promise<void> {
  if (!password.value || store.isBusy) {
    return;
  }
  const replacement = await replaceSecretKey(password.value);
  password.value = "";
  if (replacement) {
    result.value = replacement;
  }
}

async function saveEmergencyKit(): Promise<void> {
  if (await saveNewAccountEmergencyKit()) {
    hasSavedEmergencyKit.value = true;
  }
}

async function close(): Promise<void> {
  if (result.value) {
    if (!isDoneEnabled.value) {
      return;
    }
    await finishSecretKeyReplacement();
  }
  clearError();
  emit("close");
}
</script>

<template>
  <AccountDialog
    title="Replace Secret Key"
    :subtitle="result ? 'Save the new Emergency Kit now. It is the only copy outside your signed-in devices.' : 'Your master password stays the same.'"
    icon="key"
    :is-closable="!store.isBusy && (!result || isDoneEnabled)"
    data-testid="replace-secret-key-dialog"
    @close="close"
  >
    <template v-if="result">
      <div class="key" data-testid="new-secret-key" :aria-label="result.secretKey">
        <span v-for="(part, index) in allKeyPart" :key="index" class="key-part monospace">{{ part }}</span>
      </div>
      <p v-if="result.keychainError" class="error" role="alert" data-testid="keychain-error">{{ result.keychainError }}</p>
      <ul class="consequences" data-testid="replace-secret-key-done">
        <li>Your old Secret Key and Emergency Kit no longer work.</li>
        <li>{{ describeSignedOutDevices(result.signedOutDeviceCount) }} Sign in on them with your master password and the new Secret Key.</li>
      </ul>
      <div class="row key-actions">
        <button @click="copyText(result.secretKey, 'Secret Key')"><AppIcon name="copy" /> Copy</button>
        <button class="primary" data-testid="save-new-emergency-kit" @click="saveEmergencyKit"><AppIcon name="note" /> Save Emergency Kit…</button>
      </div>
      <label v-if="!hasSavedEmergencyKit" class="check-row">
        <input v-model="hasConfirmedSaved" type="checkbox" name="hasConfirmedSaved" />
        <span>I have written down or stored the new Secret Key somewhere safe</span>
      </label>
      <p v-else class="hint muted" data-testid="new-emergency-kit-saved"><AppIcon name="check" :size="12" /> Emergency Kit saved.</p>
      <div class="row actions">
        <button class="primary" :disabled="!isDoneEnabled" data-testid="replace-secret-key-close" @click="close">Done</button>
      </div>
    </template>

    <form v-else novalidate @submit.prevent="submitReplacement">
      <ul class="consequences warning" data-testid="replace-secret-key-consequences">
        <li>
          Your other devices will be signed out. Sign in on them with your master password and the new Secret Key from the new Emergency Kit.
          Your old Emergency Kit stops working.
        </li>
        <li>Your master password stays the same. Vaults, items and the people you share with are not affected.</li>
        <li>Replace it when your Secret Key or Emergency Kit may have been seen by someone else.</li>
      </ul>
      <label>
        <span>Master password</span>
        <input v-model="password" name="replaceSecretKeyPassword" type="password" autocomplete="current-password" autofocus />
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

<style scoped>
.key {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 4px;
  margin-bottom: 14px;
  padding: 16px 12px;
  border: 1px dashed var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  user-select: text;
  -webkit-user-select: text;
}

.key-part {
  font-size: 15px;
  font-weight: 600;
}

.key-part:not(:last-child)::after {
  content: "-";
  margin-left: 4px;
  color: var(--color-text-muted);
}

.key-actions {
  margin-bottom: 12px;
}
</style>
