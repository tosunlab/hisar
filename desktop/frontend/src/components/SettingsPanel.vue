<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { disableBiometricUnlock, enableBiometricUnlock, signOut, store } from "../store";
import AppIcon from "./AppIcon.vue";

const isEnablingTouchID = ref(false);
const touchIDPassword = ref("");
const isConfirmingSignOut = ref(false);

function closeSettings(): void {
  store.isSettingsOpen = false;
  store.errorMessage = "";
}

async function submitEnableTouchID(): Promise<void> {
  if (await enableBiometricUnlock(touchIDPassword.value)) {
    isEnablingTouchID.value = false;
  }
  touchIDPassword.value = "";
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    closeSettings();
  }
}

onMounted(() => window.addEventListener("keydown", closeOnEscape));
onBeforeUnmount(() => window.removeEventListener("keydown", closeOnEscape));
</script>

<template>
  <div class="overlay" @click.self="closeSettings">
    <div class="card settings" role="dialog" aria-modal="true" aria-labelledby="settings-title">
      <header class="row">
        <h2 id="settings-title">Settings</h2>
        <div class="spacer"></div>
        <button class="ghost icon-button" aria-label="Close settings" @click="closeSettings"><AppIcon name="close" /></button>
      </header>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <section>
        <p class="section-label">Security</p>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="fingerprint" /></span>
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
        <form v-if="isEnablingTouchID" class="enable-form" @submit.prevent="submitEnableTouchID">
          <input
            v-model="touchIDPassword"
            name="touchIDPassword"
            type="password"
            aria-label="Master password"
            placeholder="Enter your master password to turn on Touch ID"
            autocomplete="current-password"
            autofocus
            required
          />
          <button type="button" class="ghost" @click="isEnablingTouchID = false">Cancel</button>
          <button class="primary" type="submit" :disabled="store.isBusy">Turn on</button>
        </form>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="lock" /></span>
          <div class="setting-text">
            <strong>Auto-lock</strong>
            <p class="muted">Hisar locks after 10 minutes without activity. Copied values are cleared after 90 seconds.</p>
          </div>
        </div>
      </section>

      <section>
        <p class="section-label">Account</p>
        <div class="setting">
          <span class="setting-icon"><AppIcon name="user" /></span>
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
      </section>

      <p class="version muted">Hisar 0.1.0</p>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgb(0 0 0 / 55%);
}

.settings {
  width: min(560px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  background: var(--color-panel);
  box-shadow: 0 24px 64px rgb(0 0 0 / 50%);
}

h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 650;
}

header {
  margin-bottom: 16px;
}

section {
  margin-bottom: 18px;
}

.setting {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 0;
  border-bottom: 1px solid var(--color-border);
}

.setting:last-child {
  border-bottom: none;
}

.setting-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text-secondary);
}

.setting-text {
  flex: 1;
  min-width: 0;
}

.setting-text p {
  margin: 2px 0 0;
  font-size: 12px;
}

.enable-form {
  display: flex;
  gap: 8px;
  padding: 0 0 12px 44px;
}

.version {
  margin: 0;
  font-size: 11px;
}
</style>
