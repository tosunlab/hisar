<script setup lang="ts">
import { ref } from "vue";
import { disableBiometricUnlock, enableBiometricUnlock, saveEmergencyKit, setAutoLock, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { isMacPlatform } from "../util";
import ChangeMasterPasswordDialog from "./ChangeMasterPasswordDialog.vue";
import ReplaceSecretKeyDialog from "./ReplaceSecretKeyDialog.vue";

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
</script>

<template>
  <section data-testid="security-section">
    <div v-show="isRowVisible('touch-id') || isRowVisible('auto-lock')" class="settings-block">
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
                    ? "Touch ID unlocks Hisar after it locks. Your master password is still needed when Hisar starts and at least every 14 days."
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
          <button type="button" class="ghost" @click="isEnablingTouchID = false">Cancel</button>
          <button class="primary" type="submit" :disabled="store.isBusy">Turn on</button>
        </form>
        <div v-show="isRowVisible('auto-lock')" class="setting">
          <div class="setting-text">
            <strong>Auto-lock</strong>
            <p v-if="isMacPlatform()" class="muted">Hisar locks once your Mac has had no keyboard, mouse or trackpad input for this long, and when the screen locks or the Mac sleeps. Typing in a terminal or editor keeps it unlocked; SSH, command-line and agent requests alone do not. Copied values are cleared after 90 seconds.</p>
            <p v-else class="muted">Hisar locks once this computer has had no keyboard or mouse input for this long; on a desktop that does not report it, once Hisar itself has not been used for this long. Copied values are cleared after 90 seconds.</p>
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
      </div>
    </div>

    <div v-show="isRowVisible('change-master-password') || isRowVisible('replace-secret-key')" class="settings-block">
      <p class="section-label">Master password and Secret Key</p>
      <div class="settings-group">
        <div v-show="isRowVisible('change-master-password')" class="setting">
          <div class="setting-text">
            <strong>Master password</strong>
            <p class="muted">Changing it signs out your other devices. Your Secret Key and Emergency Kit stay the same.</p>
          </div>
          <button data-testid="change-master-password" @click="isChangingMasterPassword = true">Change master password…</button>
        </div>
        <div v-show="isRowVisible('replace-secret-key')" class="setting">
          <div class="setting-text">
            <strong>Secret Key</strong>
            <p class="muted">Replace it if your Secret Key or Emergency Kit may have been seen by someone else. You get a new Emergency Kit, and your other devices are signed out.</p>
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
          <button v-if="!isSavingEmergencyKit" data-testid="save-emergency-kit" @click="isSavingEmergencyKit = true">Save PDF</button>
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
          <button type="button" class="ghost" @click="isSavingEmergencyKit = false">Cancel</button>
          <button class="primary" type="submit" :disabled="store.isBusy">Save</button>
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
</style>
