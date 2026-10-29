<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { api, getErrorMessage } from "../api";
import { authorizeEmergencyKit, closeEmergencyKit, confirmSecretKeySaved, postponeSecretKey, saveNewAccountEmergencyKit, store } from "../store";
import { continueWithBiometric, fingerprintScanPrompt, getThisDeviceName, isMacPlatform } from "../util";
import AppIcon from "./AppIcon.vue";
import CredentialField from "./CredentialField.vue";
import HisarLogo from "./HisarLogo.vue";
import SecretKeyBlock from "./SecretKeyBlock.vue";

const kit = computed(() => store.status.emergencyKit);
const device = getThisDeviceName();
const secretKey = ref("");
const password = ref("");
const usesPassword = ref(!store.status.biometricEnabled);
const wasCancelled = ref(false);
const isWaitingForFingerprint = ref(false);
let revealGeneration = 0;
const savedMessage = computed(() => ({ file: "Emergency Kit saved.", print: "Emergency Kit sent to the printer.", share: "Emergency Kit shared." }[kit.value?.savedBy ?? ""] ?? ""));
watch(() => [kit.value?.operationId, kit.value?.isAuthorized, store.recoveryEpoch, store.status.state, store.status.server, store.status.email], async () => {
  const generation = ++revealGeneration;
  secretKey.value = "";
  if (!kit.value?.isAuthorized || store.status.state !== "unlocked") return;
  try {
    const revealed = await api.revealNewAccountSecretKey();
    if (generation === revealGeneration) secretKey.value = revealed.secretKey;
  } catch (error) {
    if (generation === revealGeneration) store.errorMessage = getErrorMessage(error);
  }
}, { immediate: true });
onUnmounted(() => { revealGeneration++; secretKey.value = ""; password.value = ""; });

async function authorize(biometric = false): Promise<void> {
  if (store.isBusy) return;
  const waitingFingerprint = biometric && !isMacPlatform();
  if (waitingFingerprint) isWaitingForFingerprint.value = true;
  await authorizeEmergencyKit(biometric ? undefined : password.value);
  password.value = "";
  if (waitingFingerprint) {
    isWaitingForFingerprint.value = false;
    if (store.errorMessage.startsWith("Fingerprint check was cancelled")) store.errorMessage = "";
  }
}
function cancelFingerprint(): void {
  api.cancelTouchIDPrompt().catch(() => undefined);
}
async function saveKit(): Promise<void> {
  wasCancelled.value = false;
  const saved = await saveNewAccountEmergencyKit();
  wasCancelled.value = !saved && !store.errorMessage;
}
</script>

<template>
  <div class="page">
    <div class="title-bar"></div>
    <div class="page-body">
      <div class="card page-card secret-key-card">
        <HisarLogo :size="32" />
        <h1>{{ kit?.kind === 'replace-secret-key' ? 'Save your new Emergency Kit' : 'Save your Emergency Kit' }}</h1>
        <template v-if="!kit?.isAuthorized">
          <p>Enter your master password to show your Secret Key.</p>
          <form v-if="usesPassword" @submit.prevent="authorize()">
            <label><span>Master password</span><CredentialField><input v-model="password" name="kitPassword" type="password" autocomplete="current-password" required /></CredentialField></label>
            <button class="primary full-width" :disabled="store.isBusy">Continue</button>
          </form>
          <template v-else-if="isWaitingForFingerprint">
            <p class="muted" role="status" data-testid="fingerprint-waiting">{{ fingerprintScanPrompt() }}</p>
            <button class="ghost full-width" data-testid="cancel-fingerprint" @click="cancelFingerprint">Cancel</button>
          </template>
          <template v-else>
            <button class="primary full-width" :disabled="store.isBusy" @click="authorize(true)">{{ continueWithBiometric() }}</button>
            <button class="ghost full-width" :disabled="store.isBusy" @click="usesPassword = true">Use master password</button>
          </template>
          <button v-if="!isWaitingForFingerprint" class="ghost full-width" :disabled="store.isBusy" @click="closeEmergencyKit">Cancel</button>
        </template>
        <template v-else>
          <p class="subtitle">{{ kit.kind === 'replace-secret-key' ? 'Your Secret Key changed, so your old Emergency Kit no longer works. Save the new one.' : "Hisar created a Secret Key for your account. You need it and your master password to sign in on a new device. You don't need to remember it." }}</p>
          <p class="master-password-warning" data-testid="master-password-warning"><AppIcon name="warning" /><span>A forgotten master password can't be recovered by anyone, including Hisar.</span></p>
          <SecretKeyBlock v-if="secretKey" :secret-key="secretKey" />
          <p>The Emergency Kit holds your Secret Key and has a space to write your master password.</p>
          <p>Keep it where you can get to it without {{ device }}: print it, or save it to storage you can open from another device. Don't keep the only copy in Hisar.</p>
          <template v-if="savedMessage">
            <p role="status" data-testid="kit-saved">{{ savedMessage }}</p>
            <p>A copy saved only on {{ device }} is lost with it. Print it, or keep another copy somewhere else.</p>
            <button class="primary full-width" data-testid="secret-key-continue" :disabled="store.isBusy" @click="confirmSecretKeySaved">I saved my Emergency Kit</button>
          </template>
          <button class="full-width" :class="{ primary: !savedMessage }" data-testid="save-emergency-kit" :disabled="store.isBusy" @click="saveKit"><AppIcon name="note" /> {{ savedMessage ? 'Save another copy…' : 'Save Emergency Kit…' }}</button>
          <p v-if="wasCancelled" role="status">The Emergency Kit wasn't saved.</p>
          <button class="ghost full-width" data-testid="secret-key-later" :disabled="store.isBusy" @click="postponeSecretKey">Save later</button>
          <p class="hint muted">Save a copy you can reach without {{ device }}. Losing access to your signed-in devices before you save it could lock you out.</p>
        </template>
        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.secret-key-card { width: min(580px, 100%); }
.secret-key-card > button { margin-top: 8px; }
.master-password-warning { display: flex; gap: 8px; margin: 0 0 16px; padding: 9px 12px; border: 1px solid var(--color-border-strong); border-radius: var(--radius); background: var(--color-surface); color: var(--color-text); font-size: var(--font-size-13); }
.master-password-warning :deep(.icon) { flex: none; margin-top: 1px; color: var(--color-warning); }
.hint { margin: 4px 0 0; }
</style>
