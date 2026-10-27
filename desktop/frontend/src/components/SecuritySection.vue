<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";
import { api, getErrorMessage, type RevealedSecretKey } from "../api";
import { copyText, disableBiometricUnlock, enableBiometricUnlock, saveEmergencyKit, setAutoLock, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { isMacPlatform, touchIDConsent } from "../util";
import ChangeMasterPasswordDialog from "./ChangeMasterPasswordDialog.vue";
import ReplaceSecretKeyDialog from "./ReplaceSecretKeyDialog.vue";

const props = withDefaults(defineProps<{ isActive?: boolean }>(), { isActive: true });

type SecretKeyDisplay = { kind: "closed" } | { kind: "password"; isPending: boolean } | { kind: "revealed"; value: RevealedSecretKey };
const secretKeyDisplay = ref<SecretKeyDisplay>({ kind: "closed" });
const revealPassword = ref("");
let revealGeneration = 0;
const allAutoLockOption = [
  { seconds: 30, label: "30 seconds" },
  { seconds: 60, label: "1 minute" },
  { seconds: 300, label: "5 minutes" },
  { seconds: 600, label: "10 minutes" },
  { seconds: 3600, label: "1 hour" },
];
const isRowVisible = useSettingsRowVisibility();
const isEnablingTouchID = ref(false);
const touchIDPassword = ref("");
const isSavingEmergencyKit = ref(false);
const emergencyKitPassword = ref("");
const isChangingMasterPassword = ref(false);
const isReplacingSecretKey = ref(false);

function cancelEnableTouchID(): void {
  touchIDPassword.value = "";
  isEnablingTouchID.value = false;
}

function cancelSaveEmergencyKit(): void {
  emergencyKitPassword.value = "";
  isSavingEmergencyKit.value = false;
}

watch(() => props.isActive && isRowVisible("touch-id"), (visible) => {
  if (!visible) cancelEnableTouchID();
});
watch(() => props.isActive && isRowVisible("emergency-kit"), (visible) => {
  if (!visible) cancelSaveEmergencyKit();
});

async function submitEnableTouchID(): Promise<void> {
  if (await enableBiometricUnlock(touchIDPassword.value)) {
    isEnablingTouchID.value = false;
  }
  touchIDPassword.value = "";
}

async function submitSaveEmergencyKit(): Promise<void> {
  if (await saveEmergencyKit(emergencyKitPassword.value)) {
    isSavingEmergencyKit.value = false;
  }
  emergencyKitPassword.value = "";
}

function clearSecretKey(): void {
  revealGeneration++;
  secretKeyDisplay.value = { kind: "closed" };
  revealPassword.value = "";
  emergencyKitPassword.value = "";
  isSavingEmergencyKit.value = false;
}

async function submitRevealSecretKey(): Promise<void> {
  const generation = ++revealGeneration;
  store.errorMessage = "";
  secretKeyDisplay.value = { kind: "password", isPending: true };
  const password = revealPassword.value;
  revealPassword.value = "";
  try {
    const value = await api.revealSecretKey(password);
    if (generation === revealGeneration) secretKeyDisplay.value = { kind: "revealed", value };
  } catch (error) {
    if (generation === revealGeneration) {
      store.errorMessage = getErrorMessage(error);
      secretKeyDisplay.value = { kind: "password", isPending: false };
    }
  }
}

watch(
  [() => props.isActive, () => isRowVisible("emergency-kit"), () => store.status.state, () => store.status.email, () => store.status.server, () => store.recoveryEpoch],
  clearSecretKey,
  { flush: "sync" },
);
onBeforeUnmount(clearSecretKey);
</script>

<template>
  <section data-testid="security-section">
    <div v-show="isRowVisible('touch-id') || isRowVisible('auto-lock') || isRowVisible('clipboard')" class="settings-block">
      <p class="section-label">Unlock</p>
      <div class="settings-group">
        <div v-show="isRowVisible('touch-id')" class="setting">
          <div class="setting-text">
            <strong>Unlock with Touch ID</strong>
            <p class="muted">
              {{
                store.status.biometricSetupError
                  ? "Touch ID protection is pending. Unlock with your master password until setup is complete."
                  : store.status.biometricAvailable
                    ? touchIDConsent
                    : isMacPlatform()
                      ? "Touch ID is not available on this Mac."
                      : "Unlocking with a fingerprint is not available on this computer yet; Hisar unlocks with your master password."
              }}
            </p>
            <p v-if="store.status.biometricSetupError" class="error" role="status" data-testid="biometric-setup-error">{{ store.status.biometricSetupError }}</p>
          </div>
          <button
            v-if="store.status.biometricEnabled"
            data-testid="disable-touch-id"
            :disabled="store.isBusy"
            @click="disableBiometricUnlock"
          >
            {{ store.status.biometricSetupError ? "Remove saved setup" : "Turn off" }}
          </button>
          <button
            v-else-if="!isEnablingTouchID"
            data-testid="enable-touch-id"
            :disabled="!store.status.biometricAvailable"
            @click="isEnablingTouchID = true"
          >
            Turn on
          </button>
        </div>
        <form v-if="isEnablingTouchID" v-show="isRowVisible('touch-id')" class="settings-group-form" @submit.prevent="submitEnableTouchID">
          <input
            v-model="touchIDPassword"
            name="touchIDPassword"
            type="password"
            aria-label="Master password"
            placeholder="Master password to turn on Touch ID"
            autocomplete="current-password"
            autofocus
            required
          />
          <button type="button" class="ghost" @click="cancelEnableTouchID">Cancel</button>
          <button class="primary" type="submit" :disabled="store.isBusy">Turn on</button>
        </form>
        <div v-show="isRowVisible('auto-lock')" class="setting">
          <div class="setting-text">
            <strong>Auto-lock</strong>
            <p v-if="isMacPlatform()" class="muted">Hisar locks after this long without keyboard, mouse or trackpad input on this Mac, and when the screen locks.</p>
            <p v-else class="muted">Hisar locks once this computer has had no keyboard or mouse input for this long; on a desktop that does not report it, once Hisar itself has not been used for this long.</p>
          </div>
          <select
            class="auto-lock"
            aria-label="Auto-lock after"
            data-testid="auto-lock"
            :value="store.status.autoLockSeconds"
            :disabled="store.isBusy"
            @change="setAutoLock(Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="option in allAutoLockOption" :key="option.seconds" :value="option.seconds">{{ option.label }}</option>
          </select>
        </div>
        <div v-show="isRowVisible('clipboard')" class="setting" data-testid="clipboard-clearing">
          <div class="setting-text">
            <strong>Clear copied values after 90 seconds</strong>
          </div>
        </div>
      </div>
    </div>

    <div v-show="isRowVisible('change-master-password') || isRowVisible('replace-secret-key')" class="settings-block">
      <p class="section-label">Master password and Secret Key</p>
      <div class="settings-group">
        <div v-show="isRowVisible('change-master-password')" class="setting">
          <div class="setting-text">
            <strong>Master password</strong>
            <p class="muted">Signs out your other devices.</p>
          </div>
          <button data-testid="change-master-password" @click="isChangingMasterPassword = true">Change master password…</button>
        </div>
        <div v-show="isRowVisible('replace-secret-key')" class="setting">
          <div class="setting-text">
            <strong>Secret Key</strong>
            <p class="muted">Signs out your other devices.</p>
          </div>
          <button data-testid="replace-secret-key" @click="isReplacingSecretKey = true">Replace Secret Key…</button>
        </div>
      </div>
    </div>

    <div v-show="isRowVisible('emergency-kit')" class="settings-block">
      <p class="section-label">Recovery</p>
      <div class="settings-group">
        <div class="setting">
          <div class="setting-text">
            <strong>Emergency Kit</strong>
            <p class="muted">A printable PDF with your sign-in address, email and Secret Key, for signing in on a new device. It is the only way back in if you lose your devices.</p>
          </div>
          <button v-if="secretKeyDisplay.kind === 'closed'" data-testid="show-secret-key" @click="secretKeyDisplay = { kind: 'password', isPending: false }">Show Secret Key…</button>
          <button v-if="!isSavingEmergencyKit && secretKeyDisplay.kind !== 'revealed'" data-testid="save-emergency-kit" @click="isSavingEmergencyKit = true">Save Emergency Kit…</button>
        </div>
        <form v-if="secretKeyDisplay.kind === 'password'" class="settings-group-form" data-testid="reveal-secret-key-form" @submit.prevent="submitRevealSecretKey">
          <label>
            <span>Master password</span>
            <input v-model="revealPassword" name="revealSecretKeyPassword" type="password" autocomplete="current-password" required :disabled="secretKeyDisplay.isPending" />
          </label>
          <button type="button" class="ghost" @click="clearSecretKey">Cancel</button>
          <button class="primary" type="submit" :disabled="secretKeyDisplay.isPending">{{ secretKeyDisplay.isPending ? "Checking…" : "Show Secret Key" }}</button>
        </form>
        <div v-if="secretKeyDisplay.kind === 'revealed'" class="revealed-key" data-testid="revealed-secret-key">
          <div class="key" :aria-label="secretKeyDisplay.value.secretKey">
            <span v-for="(part, index) in secretKeyDisplay.value.secretKey.split('-')" :key="index" class="key-part monospace">{{ part }}</span>
          </div>
          <img class="sign-in-qr" :src="`data:image/png;base64,${secretKeyDisplay.value.qrCode}`" alt="Scan with Hisar on your phone to sign in" data-testid="secret-key-qr" />
          <div class="row">
            <button data-testid="copy-secret-key" @click="copyText(secretKeyDisplay.value.secretKey, 'Secret Key')">Copy Secret Key</button>
          <button v-if="!isSavingEmergencyKit" data-testid="save-emergency-kit" @click="isSavingEmergencyKit = true">Save Emergency Kit…</button>
            <button class="ghost" data-testid="hide-secret-key" @click="clearSecretKey">Hide Secret Key</button>
          </div>
        </div>
        <form v-if="isSavingEmergencyKit" class="settings-group-form" @submit.prevent="submitSaveEmergencyKit">
          <input
            v-model="emergencyKitPassword"
            name="emergencyKitPassword"
            type="password"
            aria-label="Master password"
            placeholder="Master password to save the Emergency Kit"
            autocomplete="current-password"
            autofocus
            required
          />
          <button type="button" class="ghost" @click="cancelSaveEmergencyKit">Cancel</button>
          <button class="primary" type="submit" :disabled="store.isBusy">Save Emergency Kit…</button>
        </form>
      </div>
    </div>
    <ChangeMasterPasswordDialog v-if="isChangingMasterPassword" @close="isChangingMasterPassword = false" />
    <ReplaceSecretKeyDialog v-if="isReplacingSecretKey" @close="isReplacingSecretKey = false" />
  </section>
</template>

<style scoped>
.auto-lock {
  flex: none;
  width: auto;
}

.revealed-key {
  padding: 16px;
}

.key {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 12px;
  border: 1px dashed var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  user-select: text;
  -webkit-user-select: text;
}

.key-part {
  padding: 4px 6px;
  border-radius: var(--radius-small);
  background: var(--color-surface-raised);
  font-size: var(--font-size-15);
}

.sign-in-qr {
  display: block;
  width: 240px;
  max-width: 100%;
  height: auto;
  margin: 16px auto;
}
</style>
