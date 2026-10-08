<script setup lang="ts">
import { ref } from "vue";
import { signOut, store, unlock } from "../store";
import { getInitials } from "../util";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";

const password = ref("");
const isConfirmingSignOut = ref(false);

async function submitUnlock(): Promise<void> {
  await unlock(password.value);
  password.value = "";
}
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
        <form class="unlock-form" @submit.prevent="submitUnlock">
          <span class="password-field">
            <AppIcon name="lock" />
            <input
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
