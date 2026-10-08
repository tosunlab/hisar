<script setup lang="ts">
import { computed, ref } from "vue";
import { clearError, signIn, signUp, store } from "../store";

type Mode = "sign-in" | "create-account";

const mode = ref<Mode>("sign-in");
const server = ref(store.status.server || "https://tryhisar.com");
const email = ref(store.status.email);
const secretKey = ref("");
const password = ref("");
const inviteLink = ref("");
const name = ref("");
const repeatedPassword = ref("");

const passwordMismatch = computed(
  () => mode.value === "create-account" && repeatedPassword.value !== "" && password.value !== repeatedPassword.value,
);

function switchMode(nextMode: Mode): void {
  mode.value = nextMode;
  password.value = "";
  repeatedPassword.value = "";
  clearError();
}

async function submitSignIn(): Promise<void> {
  const succeeded = await signIn(server.value, email.value, secretKey.value, password.value);
  if (succeeded) {
    password.value = "";
    secretKey.value = "";
  }
}

async function submitCreateAccount(): Promise<void> {
  if (password.value !== repeatedPassword.value) {
    return;
  }
  const succeeded = await signUp(inviteLink.value, email.value, name.value, password.value);
  if (succeeded) {
    password.value = "";
    repeatedPassword.value = "";
  }
}
</script>

<template>
  <div class="centered-page">
    <div class="title-bar" style="width: 100%"></div>
    <div class="card">
      <h1>Hisar</h1>
      <p class="subtitle muted">
        {{ mode === "sign-in" ? "Sign in to your account on this device." : "Create your account from an invite." }}
      </p>

      <div class="row mode-switch" role="tablist">
        <button role="tab" :aria-selected="mode === 'sign-in'" :class="{ primary: mode === 'sign-in' }" @click="switchMode('sign-in')">
          Sign in
        </button>
        <button
          role="tab"
          :aria-selected="mode === 'create-account'"
          :class="{ primary: mode === 'create-account' }"
          @click="switchMode('create-account')"
        >
          Create account
        </button>
      </div>

      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

      <form v-if="mode === 'sign-in'" @submit.prevent="submitSignIn">
        <label><span>Server</span><input v-model="server" name="server" autocomplete="url" required /></label>
        <label><span>Email</span><input v-model="email" name="email" type="email" autocomplete="username" required /></label>
        <label>
          <span>Secret Key</span>
          <input v-model="secretKey" name="secretKey" class="monospace" placeholder="H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX" autocomplete="off" spellcheck="false" required />
        </label>
        <label>
          <span>Master password</span>
          <input v-model="password" name="password" type="password" autocomplete="current-password" required />
        </label>
        <button class="primary" type="submit" :disabled="store.isBusy">{{ store.isBusy ? "Signing in…" : "Sign in" }}</button>
      </form>

      <form v-else @submit.prevent="submitCreateAccount">
        <label>
          <span>Invite link</span>
          <input v-model="inviteLink" name="inviteLink" placeholder="https://tryhisar.com/invite/…" autocomplete="off" required />
        </label>
        <label><span>Email</span><input v-model="email" name="email" type="email" autocomplete="username" required /></label>
        <label><span>Your name</span><input v-model="name" name="name" autocomplete="name" required /></label>
        <label>
          <span>Master password (at least 10 characters)</span>
          <input v-model="password" name="password" type="password" autocomplete="new-password" minlength="10" required />
        </label>
        <label>
          <span>Repeat master password</span>
          <input v-model="repeatedPassword" name="repeatedPassword" type="password" autocomplete="new-password" required />
        </label>
        <p v-if="passwordMismatch" class="error">The passwords do not match.</p>
        <button class="primary" type="submit" :disabled="store.isBusy || passwordMismatch">
          {{ store.isBusy ? "Creating account…" : "Create account" }}
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.mode-switch {
  margin-bottom: 16px;
}

.mode-switch button {
  flex: 1;
}

form button[type="submit"] {
  width: 100%;
  margin-top: 4px;
}
</style>
