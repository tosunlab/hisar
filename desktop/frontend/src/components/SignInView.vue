<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { api, getErrorMessage } from "../api";
import { clearError, dismissSignedOutReason, signIn, signUp, signUpWithoutInvite, store } from "../store";
import { getMasterPasswordFeedback, getThisDeviceName, isMacPlatform } from "../util";
import AppIcon, { type IconName } from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";
import StrengthMeter from "./StrengthMeter.vue";

type Mode = "sign-in" | "create-account";

const defaultServer = "eu.tryhisar.com";
const tagline = "Passwords, SSH keys and secrets for developers, end-to-end encrypted.";

function getDisplayServer(serverURL: string): string {
  return (serverURL || defaultServer).replace(/^https:\/\//, "").replace(/\/$/, "");
}

const mode = ref<Mode>("sign-in");
const server = ref(getDisplayServer(store.status.server));
// The server stays behind "Use a different server…" even when it is a remembered self-hosted one; the hint names it.
const isServerShown = ref(false);
const email = ref(store.status.email);
// At launch this view opens before the backend's first status, which brings the remembered server and email.
watch(
  () => [store.status.server, store.status.email],
  ([statusServer, statusEmail]) => {
    if (!isServerShown.value) {
      server.value = getDisplayServer(statusServer);
    }
    if (!email.value) {
      email.value = statusEmail;
    }
  },
);
const secretKey = ref("");
const password = ref("");
const inviteCode = ref("");
const pastedInviteLink = ref("");
const name = ref("");
const repeatedPassword = ref("");
const serverInput = ref<HTMLInputElement | null>(null);
const emailInput = ref<HTMLInputElement | null>(null);
const secretKeyInput = ref<HTMLInputElement | null>(null);
const passwordInput = ref<HTMLInputElement | null>(null);
const isOpeningKit = ref(false);

async function openEmergencyKit(): Promise<void> {
  clearError();
  isOpeningKit.value = true;
  try {
    const fields = await api.openEmergencyKit();
    if (!fields) return;
    server.value = getDisplayServer(fields.server);
    email.value = fields.email;
    secretKey.value = fields.secretKey;
    password.value = "";
    await nextTick();
    passwordInput.value?.focus();
  } catch (error) {
    store.errorMessage = getErrorMessage(error);
  } finally {
    isOpeningKit.value = false;
  }
}

onMounted(() => {
  const firstEmpty = [emailInput.value, secretKeyInput.value, passwordInput.value].find((input) => input && !input.value);
  firstEmpty?.focus();
});

watch(isServerShown, async (isShown) => {
  if (isShown) {
    await nextTick();
    serverInput.value?.focus();
  }
});

const passwordMismatch = computed(
  () => mode.value !== "sign-in" && repeatedPassword.value !== "" && password.value !== repeatedPassword.value,
);
const passwordFeedback = computed(() => getMasterPasswordFeedback(mode.value === "sign-in" ? "" : password.value));
const allSubtitle: Record<Mode, string> = {
  "sign-in": `Sign in to Hisar on ${getThisDeviceName()}.`,
  "create-account": "Enter the invite code you received, or paste the whole invite link. Some servers let anyone sign up without one.",
};

type Feature = { icon: IconName; title: string; text: string };
const allSignInFeature: Feature[] = [
  {
    icon: "key",
    title: isMacPlatform() ? "SSH agent with Touch ID" : "SSH agent with approvals",
    text: "SSH and commit signing use keys kept in Hisar, with your approval. Private keys never leave Hisar.",
  },
  {
    icon: "terminal",
    title: "Made for the terminal",
    text: "The hisar command-line tool reads passwords and one-time codes, and uses SSH keys through the agent, so scripts never store them.",
  },
  { icon: "shield", title: "End-to-end encrypted", text: "Items are encrypted on your devices and never reach Hisar's servers unencrypted." },
];
const allCreateAccountFeature: Feature[] = [
  {
    icon: "shield",
    title: "Safe even if the server is breached",
    text: "Everything is encrypted before it leaves your devices. A stolen database is unreadable without your two secrets.",
  },
  {
    icon: "key",
    title: "Two secrets, only yours",
    text: "Your master password and a Secret Key you get next. Neither is ever sent to the server.",
  },
  { icon: "note", title: "An Emergency Kit to keep", text: "Print it or store it offline. With it, you can sign in again on any new device." },
];
const allBrandFeature = computed(() => (mode.value === "create-account" ? allCreateAccountFeature : allSignInFeature));

function readInviteCode(): void {
  const match = inviteCode.value.trim().match(/\/invite\/([A-Za-z0-9_-]+)\/?$/);
  if (match) {
    pastedInviteLink.value = inviteCode.value.trim();
    inviteCode.value = match[1];
  }
}

const inviteServer = computed(() => {
  const invite = getInvite();
  if (!invite.includes("/invite/")) {
    return "";
  }
  const host = invite.replace(/^https?:\/\//, "").split("/")[0];
  return host === defaultServer ? "" : host;
});

function getInvite(): string {
  return pastedInviteLink.value.endsWith(`/invite/${inviteCode.value}`) ? pastedInviteLink.value : inviteCode.value;
}

// Without an invite the account is created on the chosen server, which must have opened registration.
const hasInvite = computed(() => inviteCode.value.trim() !== "");

// An invite opened through hisar://invite/… arrives as the bare code, or as the invite link on another server.
watch(
  () => store.pendingInvite,
  (invite) => {
    if (!invite) {
      return;
    }
    switchMode("create-account");
    inviteCode.value = invite;
    readInviteCode();
    store.pendingInvite = "";
  },
  { immediate: true },
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
  if (password.value !== repeatedPassword.value || passwordFeedback.value.isBlocking) {
    return;
  }
  const succeeded = hasInvite.value
    ? await signUp(getInvite(), email.value, name.value, password.value)
    : await signUpWithoutInvite(server.value, email.value, name.value, password.value);
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
        <h2 data-testid="brand-title">{{ tagline }}</h2>
        <ul class="feature-list">
          <li v-for="feature in allBrandFeature" :key="feature.title">
            <span class="feature-icon"><AppIcon :name="feature.icon" /></span>
            <div>
              <strong>{{ feature.title }}</strong>
              <p>{{ feature.text }}</p>
            </div>
          </li>
        </ul>
      </div>
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

        <h1 v-if="mode === 'create-account'">Create your account</h1>
        <p class="subtitle">{{ allSubtitle[mode] }}</p>

        <div v-if="store.status.signedOutReason" class="signed-out-notice" role="status" data-testid="signed-out-reason">
          <AppIcon name="shield" />
          <p>{{ store.status.signedOutReason }}</p>
          <button type="button" class="ghost" data-testid="dismiss-signed-out-reason" @click="dismissSignedOutReason">OK</button>
        </div>
        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

        <form v-if="mode === 'sign-in'" @submit.prevent="submitSignIn">
          <div v-if="isMacPlatform()" class="kit-import">
            <button type="button" class="ghost" data-testid="open-emergency-kit" :disabled="store.isBusy || isOpeningKit" @click="openEmergencyKit">
              {{ isOpeningKit ? "Opening…" : "Open Emergency Kit…" }}
            </button>
            <p class="hint muted">The QR code fills in your server, email and Secret Key.</p>
          </div>
          <label v-if="isServerShown">
            <span>Server</span>
            <span class="input-with-icon"><AppIcon name="globe" /><input ref="serverInput" v-model="server" name="server" autocomplete="url" required /></span>
          </label>
          <label>
            <span>Email</span>
            <span class="input-with-icon"><AppIcon name="user" /><input ref="emailInput" v-model="email" name="email" type="email" autocomplete="username" placeholder="you@example.com" required /></span>
          </label>
          <label>
            <span>Secret Key</span>
            <span class="input-with-icon">
              <AppIcon name="key" />
              <input ref="secretKeyInput" v-model="secretKey" name="secretKey" class="monospace" placeholder="H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX" autocomplete="off" spellcheck="false" required />
            </span>
          </label>
          <label>
            <span>Master password</span>
            <span class="input-with-icon"><AppIcon name="lock" /><input ref="passwordInput" v-model="password" name="password" type="password" autocomplete="current-password" required /></span>
          </label>
          <button class="primary full-width submit" type="submit" :disabled="store.isBusy || isOpeningKit">{{ store.isBusy ? "Signing in…" : "Sign in" }}</button>
          <p class="hint muted" data-testid="secret-key-note">Lost your Secret Key? It is on your Emergency Kit. If you can unlock another signed-in device with your master password, save a new Emergency Kit there in Settings.</p>
          <p v-if="!isServerShown" class="hint">
            <span class="muted" data-testid="sign-in-server">Signs in to {{ server }}.</span>
            {{ ' ' }}
            <button type="button" class="link-button" data-testid="use-different-server" @click="isServerShown = true">Use a different server…</button>
          </p>
        </form>

        <form v-else @submit.prevent="submitCreateAccount">
          <label>
            <span>Invite code or link (optional)</span>
            <span class="input-with-icon">
              <AppIcon name="external" />
              <input
                v-model="inviteCode"
                name="inviteCode"
                placeholder="Invite code or link"
                autocomplete="off"
                spellcheck="false"
                @input="readInviteCode"
              />
            </span>
            <span v-if="inviteServer" class="muted invite-server" data-testid="invite-server">Server: {{ inviteServer }}</span>
            <span v-else-if="!hasInvite" class="muted invite-server" data-testid="invite-hint">Leave it empty if the server lets anyone sign up.</span>
          </label>
          <label v-if="isServerShown && !hasInvite">
            <span>Server</span>
            <span class="input-with-icon"><AppIcon name="globe" /><input ref="serverInput" v-model="server" name="server" autocomplete="url" required /></span>
          </label>
          <div class="two-columns">
            <label>
              <span>Name</span>
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
            <StrengthMeter :password="password" />
            <span class="password-feedback" :class="passwordFeedback.isBlocking ? 'blocking' : 'muted'" data-testid="password-feedback">{{ passwordFeedback.message }}</span>
          </label>
          <label>
            <span>Repeat master password</span>
            <input v-model="repeatedPassword" name="repeatedPassword" type="password" autocomplete="new-password" required />
          </label>
          <p v-if="passwordMismatch" class="error">The passwords do not match.</p>
          <button class="primary full-width submit" type="submit" :disabled="store.isBusy || passwordMismatch || passwordFeedback.isBlocking">
            {{ store.isBusy ? "Creating account…" : "Create account" }}
          </button>
          <p class="hint muted" data-testid="legal-links">By creating an account you agree to the <a href="https://eu.tryhisar.com/terms" target="_blank">Terms</a> and <a href="https://eu.tryhisar.com/privacy" target="_blank">Privacy Policy</a>.</p>
          <p v-if="!isServerShown && !hasInvite" class="hint">
            <span class="muted" data-testid="create-account-server">Creates the account on {{ server }}.</span>
            {{ ' ' }}
            <button type="button" class="link-button" data-testid="use-different-server" @click="isServerShown = true">Use a different server…</button>
          </p>
        </form>
      </div>
    </main>
  </div>
</template>

<style scoped>
.signed-out-notice {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 0 0 14px;
  padding: 10px 12px;
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius);
  background: var(--color-warning-background);
  color: var(--color-warning-text);
}
.signed-out-notice p {
  flex: 1;
  margin: 0;
  font-size: var(--font-size-12);
  line-height: 1.45;
}
.signed-out-notice button {
  flex: none;
  padding: 2px 10px;
  color: inherit;
}
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
    radial-gradient(600px 400px at 0% 0%, var(--color-page-glow) 0%, transparent 70%),
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
  font-size: var(--font-size-22);
  font-weight: 600;
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
  font-size: var(--font-size-22);
  font-weight: 600;
  letter-spacing: -0.01em;
}

.subtitle {
  margin: 0 0 22px;
  color: var(--color-text-muted);
}

.segmented + .subtitle {
  margin-top: 16px;
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
  box-shadow: 0 1px 2px var(--color-shadow);
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
  font-size: var(--font-size-12);
  text-align: center;
}

.link-button {
  padding: 0;
  border: none;
  background: none;
  color: var(--color-text-muted);
  font-size: var(--font-size-12);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.link-button:hover {
  color: var(--color-text);
}

.password-feedback {
  display: block;
  margin-top: 6px;
  font-size: var(--font-size-12);
  font-weight: 400;
}

.invite-server {
  display: block;
  margin-top: 6px;
  font-size: var(--font-size-12);
  font-weight: 400;
}

.password-feedback.blocking {
  color: var(--color-danger);
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
