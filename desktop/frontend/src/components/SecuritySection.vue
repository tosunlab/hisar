<script setup lang="ts">
import { ref } from "vue";
import { disableBiometricUnlock, enableBiometricUnlock, saveEmergencyKit, setAutoLock, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";

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
                store.status.biometricAvailable
                  ? "Touch ID unlocks Hisar after it locks. Your master password is still needed when Hisar starts and at least every 14 days."
                  : "Touch ID is not available on this Mac."
              }}
            </p>
          </div>
          <button
            v-if="store.status.biometricEnabled"
            data-testid="disable-touch-id"
            :disabled="store.isBusy"
            @click="disableBiometricUnlock"
          >
            Turn off
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
            <p class="muted">Hisar locks once your Mac has had no keyboard, mouse or trackpad input for this long, and when the screen locks or the Mac sleeps. Typing in a terminal or editor keeps it unlocked; SSH, command-line and agent requests alone do not. Copied values are cleared after 90 seconds.</p>
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
  </section>
</template>

<style scoped>
.auto-lock {
  flex: none;
  width: auto;
}
</style>
