<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { api } from "../api";
import { signOut, store, unlock, unlockWithBiometric } from "../store";
import { getInitials } from "../util";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";

type Mode = "touch-id" | "password";

const mode = ref<Mode>(store.status.biometricReady ? "touch-id" : "password");
const password = ref("");
const isConfirmingSignOut = ref(false);
const isWaitingForTouchID = ref(false);
const promptSlot = ref<HTMLElement | null>(null);
const passwordInput = ref<HTMLInputElement | null>(null);

function sendPromptFrame(): void {
  const slot = promptSlot.value;
  if (!slot) {
    return;
  }
  const rectangle = slot.getBoundingClientRect();
  api.setTouchIDPromptFrame(rectangle.x, rectangle.y, rectangle.width, rectangle.height).catch(() => undefined);
}

async function startTouchID(): Promise<void> {
  mode.value = "touch-id";
  await nextTick();
  sendPromptFrame();
  isWaitingForTouchID.value = true;
  const isUnlocked = await unlockWithBiometric();
  isWaitingForTouchID.value = false;
  api.clearTouchIDPromptFrame().catch(() => undefined);
  if (!isUnlocked && mode.value === "touch-id") {
    switchToPassword();
  }
}

function switchToPassword(): void {
  if (isWaitingForTouchID.value) {
    api.cancelTouchIDPrompt().catch(() => undefined);
  }
  mode.value = "password";
  nextTick(() => passwordInput.value?.focus());
}

async function submitUnlock(): Promise<void> {
  await unlock(password.value);
  password.value = "";
}

onMounted(() => {
  window.addEventListener("resize", sendPromptFrame);
  if (mode.value === "touch-id") {
    startTouchID();
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", sendPromptFrame);
  if (isWaitingForTouchID.value) {
    api.cancelTouchIDPrompt().catch(() => undefined);
  }
  api.clearTouchIDPromptFrame().catch(() => undefined);
});
</script>

<template>
  <div class="page">
    <div class="title-bar"></div>
    <div class="page-body">
      <div class="unlock">
        <HisarLogo :size="44" />
        <h1>Hisar is locked</h1>
        <div class="account">
          <span class="avatar">{{ getInitials(store.status.name || store.status.email) }}</span>
          <div>
            <strong>{{ store.status.name || store.status.email }}</strong>
            <p class="muted">{{ store.status.email }} · {{ store.status.server.replace(/^https?:\/\//, "") }}</p>
          </div>
        </div>
        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

        <section v-if="mode === 'touch-id'" class="card touch-id" aria-live="polite">
          <div ref="promptSlot" class="prompt-slot" data-testid="touch-id-slot">
            <AppIcon v-if="!api.hasNativeTouchIDView" name="fingerprint" :size="56" />
          </div>
          <strong>Touch ID to unlock</strong>
          <p class="muted">Place your finger on Touch ID.</p>
          <button class="link" data-testid="use-master-password" @click="switchToPassword">Use master password</button>
        </section>

        <template v-else>
          <form class="unlock-form" @submit.prevent="submitUnlock">
            <span class="password-field">
              <AppIcon name="lock" />
              <input
                ref="passwordInput"
                v-model="password"
                name="password"
                type="password"
                aria-label="Master password"
                placeholder="Master password"
                autocomplete="current-password"
                autofocus
                required
              />
            </span>
            <button class="primary" type="submit" :disabled="store.isBusy">{{ store.isBusy ? "Unlocking…" : "Unlock" }}</button>
          </form>
          <button v-if="store.status.biometricReady" class="ghost touch-id-switch" data-testid="use-touch-id" @click="startTouchID">
            <AppIcon name="fingerprint" /> Use Touch ID
          </button>
        </template>

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
.unlock {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: min(380px, 100%);
}

h1 {
  margin: 18px 0 0;
  font-size: 20px;
  font-weight: 650;
  letter-spacing: -0.01em;
}

.account {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 20px 0 22px;
  padding: 10px 14px 10px 10px;
  border: 1px solid var(--color-border);
  border-radius: 28px;
  background: var(--color-surface);
}

.account p {
  margin: 2px 0 0;
  font-size: 12px;
}

.avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--color-selected);
  font-weight: 600;
}

.touch-id {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
  padding: 26px 20px 20px;
  text-align: center;
}

.prompt-slot {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 72px;
  height: 72px;
  margin-bottom: 14px;
  color: var(--color-text-secondary);
}

.touch-id strong {
  font-size: 15px;
}

.touch-id p {
  margin: 4px 0 16px;
}

.unlock-form {
  display: flex;
  gap: 8px;
  width: 100%;
}

.password-field {
  position: relative;
  flex: 1;
  color: var(--color-text-muted);
}

.password-field :deep(.icon) {
  position: absolute;
  top: 50%;
  left: 11px;
  transform: translateY(-50%);
}

.password-field input {
  padding: 9px 10px 9px 34px;
  color: var(--color-text);
}

.touch-id-switch {
  margin-top: 12px;
}

.error {
  width: 100%;
}

.sign-out {
  margin-top: 28px;
  text-align: center;
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
