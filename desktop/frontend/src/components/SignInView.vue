<script setup lang="ts">
import { computed, ref } from "vue";
import { clearError, signIn, signUp, store } from "../store";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";

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
  <div class="sign-in">
    <aside class="brand">
      <div class="title-bar"></div>
      <div class="brand-content">
        <HisarLogo :size="36" show-wordmark />
        <h2>Your family's passwords,<br />locked in your own fortress.</h2>
        <ul class="feature-list">
          <li>
            <span class="feature-icon"><AppIcon name="shield" /></span>
            <div>
              <strong>End-to-end encrypted</strong>
              <p>Items are encrypted on your devices. The server only ever stores ciphertext.</p>
            </div>
          </li>
          <li>
            <span class="feature-icon"><AppIcon name="key" /></span>
            <div>
              <strong>Two secrets</strong>
              <p>Your master password and Secret Key are both needed. Neither ever leaves your devices.</p>
            </div>
          </li>
          <li>
            <span class="feature-icon"><AppIcon name="users" /></span>
            <div>
              <strong>Shared vaults</strong>
              <p>Share a Family vault with the people you trust, on your own server.</p>
            </div>
          </li>
        </ul>
      </div>
      <p class="brand-footer muted">Self-hosted · Zero-knowledge · Post-quantum</p>
    </aside>

    <main class="form-side">
      <div class="title-bar"></div>
      <div class="form-container">
        <div class="segmented" role="tablist">
          <button role="tab" :aria-selected="mode === 'sign-in'" :class="{ active: mode === 'sign-in' }" @click="switchMode('sign-in')">
            Sign in
          </button>
          <button
            role="tab"
            :aria-selected="mode === 'create-account'"
            :class="{ active: mode === 'create-account' }"
            @click="switchMode('create-account')"
          >
            Create account
          </button>
        </div>

        <h1>{{ mode === "sign-in" ? "Welcome back" : "Join your family's Hisar" }}</h1>
        <p class="subtitle">
          {{ mode === "sign-in" ? "Sign in to use Hisar on this device." : "Use the invite link your Hisar admin sent you." }}
        </p>

        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

        <form v-if="mode === 'sign-in'" @submit.prevent="submitSignIn">
          <label>
            <span>Server</span>
            <span class="input-with-icon"><AppIcon name="globe" /><input v-model="server" name="server" autocomplete="url" required /></span>
          </label>
          <label>
            <span>Email</span>
            <span class="input-with-icon"><AppIcon name="user" /><input v-model="email" name="email" type="email" autocomplete="username" placeholder="you@example.com" required /></span>
          </label>
          <label>
            <span>Secret Key</span>
            <span class="input-with-icon">
              <AppIcon name="key" />
              <input v-model="secretKey" name="secretKey" class="monospace" placeholder="H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX" autocomplete="off" spellcheck="false" required />
            </span>
          </label>
          <label>
            <span>Master password</span>
            <span class="input-with-icon"><AppIcon name="lock" /><input v-model="password" name="password" type="password" autocomplete="current-password" required /></span>
          </label>
          <button class="primary full-width submit" type="submit" :disabled="store.isBusy">{{ store.isBusy ? "Signing in…" : "Sign in" }}</button>
          <p class="hint muted">Your Secret Key is on your Emergency Kit, or on a device where you already use Hisar.</p>
        </form>

        <form v-else @submit.prevent="submitCreateAccount">
          <label>
            <span>Invite link</span>
            <span class="input-with-icon"><AppIcon name="external" /><input v-model="inviteLink" name="inviteLink" placeholder="https://tryhisar.com/invite/…" autocomplete="off" required /></span>
          </label>
          <div class="two-columns">
            <label>
              <span>Your name</span>
              <input v-model="name" name="name" autocomplete="name" required />
            </label>
            <label>
              <span>Email</span>
              <input v-model="email" name="email" type="email" autocomplete="username" required />
            </label>
          </div>
          <label>
            <span>Master password</span>
            <input v-model="password" name="password" type="password" autocomplete="new-password" minlength="10" placeholder="At least 10 characters" required />
          </label>
          <label>
            <span>Repeat master password</span>
            <input v-model="repeatedPassword" name="repeatedPassword" type="password" autocomplete="new-password" required />
          </label>
          <p v-if="passwordMismatch" class="error">The passwords do not match.</p>
          <button class="primary full-width submit" type="submit" :disabled="store.isBusy || passwordMismatch">
            {{ store.isBusy ? "Creating account…" : "Create account" }}
          </button>
          <p class="hint muted">Nobody can reset your master password for you. Pick one you will remember.</p>
        </form>
      </div>
    </main>
  </div>
</template>

<style scoped>
.sign-in {
  display: grid;
  grid-template-columns: minmax(300px, 42%) 1fr;
  height: 100%;
}

.brand {
  display: flex;
  flex-direction: column;
  padding: 0 40px 28px;
  border-right: 1px solid var(--color-border);
  background:
    radial-gradient(600px 400px at 0% 0%, #1b1e23 0%, transparent 70%),
    var(--color-sidebar);
}

.brand-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.brand h2 {
  margin: 28px 0 32px;
  font-size: 24px;
  font-weight: 650;
  line-height: 1.25;
  letter-spacing: -0.02em;
}

.feature-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.feature-list li {
  display: flex;
  gap: 14px;
}

.feature-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text-secondary);
}

.feature-list strong {
  font-weight: 600;
}

.feature-list p {
  margin: 2px 0 0;
  color: var(--color-text-muted);
}

.brand-footer {
  margin: 0;
  font-size: 12px;
}

.form-side {
  display: flex;
  flex-direction: column;
  overflow-y: auto;
}

.form-container {
  width: min(380px, 100% - 48px);
  margin: auto;
  padding-bottom: 48px;
}

h1 {
  margin: 24px 0 4px;
  font-size: 22px;
  font-weight: 650;
  letter-spacing: -0.01em;
}

.subtitle {
  margin: 0 0 22px;
  color: var(--color-text-muted);
}

.segmented {
  display: grid;
  grid-template-columns: 1fr 1fr;
  padding: 3px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.segmented button {
  border: none;
  background: transparent;
  color: var(--color-text-muted);
}

.segmented button.active {
  background: var(--color-selected);
  color: var(--color-text);
  box-shadow: 0 1px 2px rgb(0 0 0 / 40%);
}

.input-with-icon {
  position: relative;
  display: block;
  color: var(--color-text-muted);
}

.input-with-icon :deep(.icon) {
  position: absolute;
  top: 50%;
  left: 11px;
  transform: translateY(-50%);
  pointer-events: none;
}

.input-with-icon input {
  padding-left: 34px;
  color: var(--color-text);
}

.two-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.submit {
  margin-top: 6px;
  padding: 9px 12px;
}

.hint {
  margin: 14px 0 0;
  font-size: 12px;
  text-align: center;
}

@media (max-width: 860px) {
  .sign-in {
    grid-template-columns: 1fr;
  }

  .brand {
    display: none;
  }
}
</style>
