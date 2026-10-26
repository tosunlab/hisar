<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { api } from "../api";
import { loadShareAwaitingUnlockQuietly, signOut, store, unlock, unlockWithBiometric } from "../store";
import { describePerson } from "../util";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";

const password = ref("");
const isConfirmingSignOut = ref(false);
const isWaitingForTouchID = ref(false);
const promptSlot = ref<HTMLElement | null>(null);
const passwordInput = ref<HTMLInputElement | null>(null);

function sendPromptFrame(): void {
  const slot = promptSlot.value;
  if (!slot || !isWaitingForTouchID.value) {
    return;
  }
  const rectangle = slot.getBoundingClientRect();
  api.setTouchIDPromptFrame(rectangle.x, rectangle.y, rectangle.width, rectangle.height).catch(() => undefined);
}

async function startTouchID(): Promise<void> {
  if (isWaitingForTouchID.value) {
    return;
  }
  isWaitingForTouchID.value = true;
  await nextTick();
  sendPromptFrame();
  await unlockWithBiometric();
  isWaitingForTouchID.value = false;
  api.clearTouchIDPromptFrame().catch(() => undefined);
  nextTick(() => passwordInput.value?.focus());
}

function cancelTouchID(): void {
  if (isWaitingForTouchID.value) {
    api.cancelTouchIDPrompt().catch(() => undefined);
  }
}

async function submitUnlock(): Promise<void> {
  cancelTouchID();
  await unlock(password.value);
  password.value = "";
}

const layoutObserver = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(sendPromptFrame);

onMounted(() => {
  loadShareAwaitingUnlockQuietly();
  window.addEventListener("focus", loadShareAwaitingUnlockQuietly);
  window.addEventListener("resize", sendPromptFrame);
  layoutObserver?.observe(document.body);
  if (store.status.biometricReady) {
    startTouchID();
  } else {
    passwordInput.value?.focus();
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("focus", loadShareAwaitingUnlockQuietly);
  store.allShareAwaitingUnlock = [];
  window.removeEventListener("resize", sendPromptFrame);
  layoutObserver?.disconnect();
  cancelTouchID();
  api.clearTouchIDPromptFrame().catch(() => undefined);
});
</script>

<template>
  <div class="page">
    <div class="title-bar"></div>
    <div class="page-body">
      <div class="unlock">
        <HisarLogo :size="32" />
        <div class="heading">
          <h1>Hisar is locked</h1>
          <p class="muted" data-testid="locked-account">{{ store.status.email }} · {{ store.status.server.replace(/^https?:\/\//, "") }}</p>
        </div>
        <ul v-if="store.allShareAwaitingUnlock.length > 0" class="awaiting-list" data-testid="share-awaiting-unlock">
          <li v-for="awaiting in store.allShareAwaitingUnlock" :key="awaiting.invitationId" class="awaiting" role="status">
            <AppIcon name="users" :size="15" />
            <span><strong>{{ describePerson(awaiting.recipientName, awaiting.recipientEmail) }}</strong> accepted your invitation to a vault — unlock to finish sharing</span>
          </li>
        </ul>
        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
        <p v-if="store.status.biometricSetupError" class="muted" role="status" data-testid="biometric-setup-error">{{ store.status.biometricSetupError }}</p>

        <form class="unlock-form" @submit.prevent="submitUnlock">
          <label class="password-field">
            <AppIcon name="lock" />
            <input
              ref="passwordInput"
              v-model="password"
              name="password"
              type="password"
              aria-label="Master password"
              placeholder="Master password"
              autocomplete="current-password"
              required
            />
            <button
              v-if="store.status.biometricReady"
              type="button"
              class="touch-id-button"
              :class="{ waiting: isWaitingForTouchID }"
              aria-label="Use Touch ID"
              data-testid="use-touch-id"
              @click="startTouchID"
            >
              <span ref="promptSlot" class="prompt-slot" data-testid="touch-id-slot">
                <AppIcon v-if="!isWaitingForTouchID || !api.hasNativeTouchIDView" name="fingerprint" />
              </span>
            </button>
          </label>
          <button class="primary" type="submit" :disabled="store.isBusy">{{ store.isBusy ? "Unlocking…" : "Unlock" }}</button>
        </form>
        <p class="muted status" aria-live="polite" data-testid="unlock-status">
          {{ isWaitingForTouchID ? "Touch ID is waiting. Or type your master password." : "" }}
        </p>

        <div class="sign-out">
          <button v-if="!isConfirmingSignOut" class="link" @click="isConfirmingSignOut = true">Sign out of this device</button>
          <div v-else class="card confirm">
            <p>You will need your Secret Key to sign in again.</p>
            <div class="row">
              <button class="ghost" @click="isConfirmingSignOut = false">Cancel</button>
              <button class="danger-solid" data-testid="confirm-sign-out" @click="signOut"><AppIcon name="signOut" /> Sign out</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.awaiting-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
  margin: 0;
  padding: 0;
  list-style: none;
}

.awaiting {
  display: flex;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text-secondary);
  font-size: var(--font-size-12);
  line-height: 1.4;
}

.awaiting strong {
  color: var(--color-text);
}

.awaiting svg {
  flex: none;
  margin-top: 1px;
  color: var(--color-text-muted);
}

.unlock {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 18px;
  width: min(360px, 100%);
}

.heading {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

h1 {
  margin: 0;
  font-size: var(--font-size-17);
  font-weight: 600;
}

.heading p {
  margin: 0;
  font-size: var(--font-size-12);
}

.unlock-form {
  display: flex;
  gap: 8px;
  width: 100%;
}

.password-field {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 8px;
  height: 36px;
  margin: 0;
  padding: 0 5px 0 11px;
  box-sizing: border-box;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text-muted);
}

.password-field:focus-within {
  border-color: var(--color-focus);
  box-shadow: 0 0 0 3px var(--color-focus-ring);
}

.password-field input,
.password-field input:hover,
.password-field input:focus {
  flex: 1;
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  box-shadow: none;
  color: var(--color-text);
  outline: none;
}

.touch-id-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: var(--color-selected);
  color: var(--color-danger);
}

.touch-id-button.waiting {
  background: transparent;
}

.prompt-slot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
}

.status {
  margin: -6px 0 0;
  font-size: var(--font-size-12);
}

.error {
  width: 100%;
  margin: 0;
}

.sign-out {
  margin-top: 6px;
  text-align: center;
  font-size: var(--font-size-12);
}

.confirm {
  padding: 14px 16px;
}

.confirm p {
  margin: 0 0 10px;
}

.confirm .row {
  justify-content: center;
}
</style>
