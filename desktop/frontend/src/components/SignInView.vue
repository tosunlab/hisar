<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { api, getErrorMessage, type SignUpPolicy } from "../api";
import { clearError, dismissSignedOutReason, signIn, signUp, signUpWithoutInvite, store } from "../store";
import { useMasterPasswordFeedback } from "../passwordStrength";
import { getThisDeviceName, isMacPlatform } from "../util";
import AppIcon, { type IconName } from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";
import StrengthMeter from "./StrengthMeter.vue";
import PendingRecoveryReminder from "./PendingRecoveryReminder.vue";
import CredentialField from "./CredentialField.vue";
import SecretKeyHelp from "./SecretKeyHelp.vue";
import { canonicalServer, defaultServer, parseSignUpInvitation, type SignUpInvitation } from "../signupPolicy";

type Mode = "welcome" | "sign-in" | "create-account";
type PolicyState = { state: "checking" } | { state: "ready"; policy: SignUpPolicy } | { state: "error"; message: string };
type FieldName = "server" | "inviteCode" | "name" | "email" | "secretKey" | "password" | "repeatedPassword";
const tagline = "Passwords, SSH keys and secrets for developers, end-to-end encrypted.";
const displayServer = (value: string) => value.replace(/^https:\/\//, "").replace(/\/$/, "");
const mode = ref<Mode>(!store.status.server && !store.pendingRecovery.length && !store.pendingInvite ? "welcome" : "sign-in");
const server = ref(displayServer(store.status.server || defaultServer));
const isServerShown = ref(false);
const email = ref(store.status.email);
const secretKey = ref("");
const password = ref("");
const inviteCode = ref("");
const invitation = ref<SignUpInvitation | null>(null);
const invitationNotice = ref("");
const invitationError = ref("");
const name = ref("");
const repeatedPassword = ref("");
const serverInput = ref<HTMLInputElement | null>(null);
const passwordInput = ref<HTMLInputElement | null>(null);
const form = ref<HTMLFormElement | null>(null);
const isOpeningKit = ref(false);
const isKitImported = ref(false);
const isHelpShown = ref(false);
const errors = ref<Partial<Record<FieldName, string>>>({});
const policy = ref<PolicyState>({ state: "checking" });
const showChecking = ref(false);
const requiresInvitationAfterRefusal = ref(false);
let policyGeneration = 0;
let policyTimer: ReturnType<typeof setTimeout> | undefined;
let checkingTimer: ReturnType<typeof setTimeout> | undefined;
const host = computed(() => displayServer(canonicalServer(server.value) || server.value));
const policyMode = computed(() => policy.value.state === "ready" ? policy.value.policy.mode : "");
const requiresInvitation = computed(() => requiresInvitationAfterRefusal.value || ["invite", "setup"].includes(policyMode.value));
const showsInviteField = computed(() => !invitation.value && (policyMode.value !== "open" || requiresInvitationAfterRefusal.value));
const showsAccountFields = computed(() => !!invitation.value || requiresInvitationAfterRefusal.value || (policy.value.state === "ready" && !requiresInvitation.value));
const isChecking = computed(() => !invitation.value && policy.value.state === "checking");
const { value: passwordFeedback, isCurrent: isPasswordFeedbackCurrent } = useMasterPasswordFeedback(() => mode.value === "create-account" ? password.value : "");
const isHostedSignup = computed(() => canonicalServer(server.value) === defaultServer);

async function checkPolicy(): Promise<void> {
  clearTimeout(policyTimer);
  clearTimeout(checkingTimer);
  if (mode.value !== "create-account") return;
  const generation = ++policyGeneration;
  const requestedServer = canonicalServer(server.value);
  policy.value = { state: "checking" };
  showChecking.value = false;
  checkingTimer = setTimeout(() => { showChecking.value = true; }, 400);
  try {
    const answer = await api.getSignUpPolicy(requestedServer || server.value);
    if (generation !== policyGeneration || requestedServer !== canonicalServer(server.value) || answer.server !== requestedServer) return;
    policy.value = { state: "ready", policy: answer };
  } catch (error) {
    if (generation === policyGeneration) policy.value = { state: "error", message: getErrorMessage(error) };
  } finally {
    if (generation === policyGeneration) clearTimeout(checkingTimer);
  }
}

watch(server, () => {
  policyGeneration++;
  clearTimeout(policyTimer);
  clearError();
  invitationError.value = "";
  requiresInvitationAfterRefusal.value = false;
  errors.value = {};
  if (invitation.value && canonicalServer(server.value) !== invitation.value.server) {
    invitationNotice.value = `This invitation is for ${displayServer(invitation.value.server)}, so Hisar removed it.`;
    invitation.value = null;
    inviteCode.value = "";
  }
  if (mode.value === "create-account") {
    policy.value = { state: "checking" };
    showChecking.value = false;
    policyTimer = setTimeout(checkPolicy, 500);
  }
}, { flush: "sync" });

function acceptInvitation(value = inviteCode.value, invitedEmail = "", invitedServer = server.value): void {
  const parsed = parseSignUpInvitation(value, invitedServer);
  if (!parsed) return;
  server.value = displayServer(parsed.server);
  invitation.value = { ...parsed, email: invitedEmail || parsed.email };
  if (invitation.value.email) email.value = invitation.value.email;
  inviteCode.value = parsed.token;
  invitationError.value = "";
  invitationNotice.value = "";
  errors.value = {};
}

function pasteInvitation(event: ClipboardEvent): void {
  const text = event.clipboardData?.getData("text") ?? "";
  if (!parseSignUpInvitation(text, server.value)) return;
  event.preventDefault();
  acceptInvitation(text);
}

function removeInvitation(): void {
  invitation.value = null;
  inviteCode.value = "";
  invitationError.value = "";
  void checkPolicy();
}

watch(() => store.pendingInvite, (link) => {
  if (!link) return;
  switchMode("create-account");
  acceptInvitation(link.invite, link.email, link.server || server.value);
  store.pendingInvite = null;
}, { immediate: true });

watch(() => [store.status.server, store.status.email], ([statusServer, statusEmail]) => {
  if (mode.value === "welcome" && (statusServer || store.pendingRecovery.length)) mode.value = "sign-in";
  if (!isServerShown.value && !invitation.value && statusServer) server.value = displayServer(statusServer);
  if (!email.value) email.value = statusEmail;
});
watch([name, email, secretKey, password, repeatedPassword, inviteCode], () => { errors.value = {}; });
watch(isServerShown, async shown => { if (shown) { await nextTick(); serverInput.value?.focus(); } });
onMounted(() => {
  if (mode.value === "sign-in") Array.from(form.value?.querySelectorAll<HTMLInputElement>("input") ?? []).find(input => !input.value)?.focus();
});
onBeforeUnmount(() => { policyGeneration++; clearTimeout(policyTimer); clearTimeout(checkingTimer); });

async function openEmergencyKit(): Promise<void> {
  clearError();
  isOpeningKit.value = true;
  try {
    const fields = await api.openEmergencyKit();
    if (!fields) return;
    server.value = displayServer(fields.server);
    email.value = fields.email;
    secretKey.value = fields.secretKey;
    password.value = "";
    errors.value = {};
    isKitImported.value = true;
    await nextTick();
    passwordInput.value?.focus();
  } catch (error) {
    store.errorMessage = getErrorMessage(error);
  } finally {
    isOpeningKit.value = false;
  }
}

async function validate(fields: [FieldName, string][]): Promise<boolean> {
  errors.value = Object.fromEntries(fields.filter(([, message]) => message));
  const first = fields.find(([, message]) => message);
  if (!first) return true;
  await nextTick();
  form.value?.querySelector<HTMLInputElement>(`[name="${first[0]}"]`)?.focus();
  return false;
}
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
  { icon: "note", title: "An Emergency Kit to keep", text: "Print it or store it offline. With it and your master password, you can sign in on a new device." },
];
const allBrandFeature = computed(() => mode.value === "sign-in" ? allSignInFeature : allCreateAccountFeature);

function switchMode(nextMode: Mode): void {
  policyGeneration++;
  clearTimeout(policyTimer);
  clearTimeout(checkingTimer);
  mode.value = nextMode;
  password.value = "";
  repeatedPassword.value = "";
  errors.value = {};
  clearError();
  if (nextMode === "create-account") void checkPolicy();
}

function moveTab(event: KeyboardEvent): void {
  const modes: Mode[] = ["sign-in", "create-account"];
  const index = modes.indexOf(mode.value);
  const nextIndex = event.key === "Home" ? 0 : event.key === "End" ? modes.length - 1 : (index + (event.key === "ArrowRight" ? 1 : -1) + modes.length) % modes.length;
  switchMode(modes[nextIndex]!);
  (event.currentTarget as HTMLElement).querySelector<HTMLButtonElement>(`#account-tab-${mode.value}`)?.focus();
}

async function submitSignIn(): Promise<void> {
  if (store.isBusy || isOpeningKit.value) return;
  if (!await validate([
    ["email", !email.value.trim() || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value) ? "Enter your email." : ""],
    ["secretKey", !secretKey.value.trim() ? "Enter your Secret Key." : ""],
    ["password", !password.value ? "Enter your master password." : ""],
  ])) return;
  if (await signIn(server.value, email.value, secretKey.value, password.value)) {
    password.value = "";
    secretKey.value = "";
  }
}

async function submitCreateAccount(): Promise<void> {
  if (store.isBusy || isChecking.value || policy.value.state === "error" && !invitation.value) return;
  if (!invitation.value && inviteCode.value.trim()) acceptInvitation();
  if (!await validate([
    ["inviteCode", !invitation.value && (requiresInvitation.value || inviteCode.value.trim()) ? "Enter your invite link or code." : ""],
    ["name", !name.value.trim() ? "Enter your name." : ""],
    ["email", !email.value.trim() || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value) ? "Enter your email." : ""],
    ["password", password.value.length < 10 ? "Use a master password with at least 10 characters." : passwordFeedback.value.isBlocking ? passwordFeedback.value.message : ""],
    ["repeatedPassword", password.value !== repeatedPassword.value ? "The master passwords don't match." : ""],
  ])) return;
  if (!isPasswordFeedbackCurrent.value) return;
  const target = invitation.value;
  const succeeded = target
    ? await signUp(`${target.server}/invite/${target.token}`, email.value, name.value, password.value)
    : await signUpWithoutInvite(server.value, email.value, name.value, password.value);
  if (succeeded) {
    password.value = "";
    repeatedPassword.value = "";
  } else if (/sign-ups are not open|registration.*closed|needs? an invitation/i.test(store.errorMessage)) {
    requiresInvitationAfterRefusal.value = true;
    invitationError.value = `Sign-ups on ${host.value} need an invitation now. Ask someone with an account there to invite you.`;
    clearError();
    void checkPolicy();
  } else if (target && /invit/i.test(store.errorMessage)) {
    invitationError.value = "This invitation can't be used. It may have expired or been used already. Ask for a new one.";
    clearError();
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
        <template v-if="mode === 'welcome'">
          <h1>Welcome to Hisar</h1>
          <p class="subtitle">{{ tagline }}</p>
          <button class="primary full-width" @click="switchMode('create-account')">Create account</button>
          <button class="ghost full-width" @click="switchMode('sign-in')">I already have an account</button>
        </template>
        <template v-else>
        <div class="segmented" role="tablist" aria-label="Account" @keydown.right.prevent="moveTab" @keydown.left.prevent="moveTab" @keydown.home.prevent="moveTab" @keydown.end.prevent="moveTab">
          <button id="account-tab-sign-in" type="button" role="tab" aria-controls="account-panel-sign-in" :tabindex="mode === 'sign-in' ? 0 : -1" :aria-selected="mode === 'sign-in'" :class="{ active: mode === 'sign-in' }" @click="switchMode('sign-in')">
            Sign in
          </button>
          <button
            id="account-tab-create-account"
            type="button"
            aria-controls="account-panel-create-account"
            :tabindex="mode === 'create-account' ? 0 : -1"
            role="tab"
            :aria-selected="mode === 'create-account'"
            :class="{ active: mode === 'create-account' }"
            @click="switchMode('create-account')"
          >
            Create account
          </button>
        </div>

        <h1>{{ mode === 'create-account' ? 'Create account' : store.status.server ? 'Welcome back' : 'Sign in' }}</h1>
        <p v-if="mode === 'sign-in'" class="subtitle">Sign in to Hisar on {{ getThisDeviceName() }}.</p>
        <PendingRecoveryReminder />

        <div v-if="store.status.signedOutReason" class="signed-out-notice" role="status" data-testid="signed-out-reason">
          <AppIcon name="shield" />
          <p>{{ store.status.signedOutReason }}</p>
          <button type="button" class="ghost" data-testid="dismiss-signed-out-reason" @click="dismissSignedOutReason">OK</button>
        </div>
        <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>

        <form v-if="mode === 'sign-in'" id="account-panel-sign-in" ref="form" role="tabpanel" aria-labelledby="account-tab-sign-in" novalidate @submit.prevent="submitSignIn">
          <div v-if="isMacPlatform()" class="kit-import">
            <button type="button" class="ghost" data-testid="open-emergency-kit" :disabled="store.isBusy || isOpeningKit" @click="openEmergencyKit">{{ isOpeningKit ? "Opening…" : "Open Emergency Kit…" }}</button>
            <p class="hint muted">The QR code fills in your server, email and Secret Key.</p>
          </div>
          <p v-if="!isKitImported" class="hint muted">Or enter the details. All fields are required.</p>
          <div v-if="isKitImported" class="invitation-card" data-testid="imported-kit">
            <strong>{{ email }} on {{ host }}</strong>
            <p>Secret Key added from your Emergency Kit.</p>
            <button type="button" class="ghost" @click="isKitImported = false">Enter different details</button>
          </div>
          <p class="hint"><span class="muted" data-testid="sign-in-server">Signs in to {{ host }}.</span> <button type="button" class="link-button" data-testid="use-different-server" @click="isServerShown = true">Use a different server…</button></p>
          <label v-if="isServerShown"><span>Server</span><input ref="serverInput" v-model="server" name="server" autocomplete="url" required /></label>
          <template v-if="!isKitImported">
            <label><span>Email</span><input v-model="email" name="email" type="email" autocomplete="username" placeholder="you@example.com" aria-label="Email, required" required :aria-invalid="!!errors.email" aria-describedby="sign-in-email-error" /></label>
            <p v-if="errors.email" id="sign-in-email-error" class="error" role="alert">{{ errors.email }}</p>
            <label><span>Secret Key</span><input v-model="secretKey" name="secretKey" class="monospace" placeholder="H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX" autocomplete="off" spellcheck="false" aria-label="Secret Key, required" required :aria-invalid="!!errors.secretKey" aria-describedby="secret-key-hint sign-in-key-error" /></label>
            <p id="secret-key-hint" class="muted">Starts with H1-. It's on your Emergency Kit.</p>
            <p v-if="errors.secretKey" id="sign-in-key-error" class="error" role="alert">{{ errors.secretKey }}</p>
          </template>
          <label><span>Master password</span><CredentialField><input ref="passwordInput" v-model="password" name="password" type="password" autocomplete="current-password" aria-label="Master password, required" required :aria-invalid="!!errors.password" aria-describedby="sign-in-password-error" /></CredentialField></label>
          <p v-if="errors.password" id="sign-in-password-error" class="error" role="alert">{{ errors.password }}</p>
          <button class="primary full-width submit" type="submit" :disabled="store.isBusy || isOpeningKit">{{ store.isBusy ? "Signing in…" : "Sign in" }}</button>
          <button type="button" class="link-button help" data-testid="secret-key-note" @click="isHelpShown = true">Can't find your Secret Key?</button>
        </form>

        <form v-else id="account-panel-create-account" ref="form" role="tabpanel" aria-labelledby="account-tab-create-account" novalidate @submit.prevent="submitCreateAccount">
          <p class="hint"><span class="muted" data-testid="create-account-server">Creates your account on {{ host }}.</span> <button type="button" class="link-button" data-testid="use-different-server" @click="isServerShown = true">Use a different server…</button></p>
          <label v-if="isServerShown"><span>Server</span><input ref="serverInput" v-model="server" name="server" autocomplete="url" required @blur="policy.state === 'checking' && checkPolicy()" /></label>
          <p v-if="invitationNotice" role="status">{{ invitationNotice }}</p>
          <p v-if="invitationError" class="error" role="alert">{{ invitationError }}</p>
          <div v-if="invitation" class="invitation-card" data-testid="invitation-card">
            <strong>{{ invitation.email ? `Invitation for ${invitation.email}` : 'Invitation' }}</strong>
            <p data-testid="invite-server">on {{ displayServer(invitation.server) }}</p>
            <button type="button" class="ghost" @click="removeInvitation">Remove invitation</button>
          </div>
          <template v-else>
            <p v-if="isChecking && showChecking" role="status"><span class="spinner"></span> Checking sign-up options on {{ host }}…</p>
            <template v-if="policy.state === 'error'">
              <p class="error" role="alert">{{ policy.message }}</p>
              <button type="button" class="primary" @click="checkPolicy">Try again</button>
            </template>
            <p v-else-if="policyMode === 'invite'">You need an invitation to create an account on {{ host }}. Ask someone with an account there to invite you.</p>
            <p v-else-if="policyMode === 'setup'">{{ host }} is still being set up. You need an invitation from its admin.</p>
            <p v-else-if="policyMode === 'unknown'" data-testid="invite-hint">This server doesn't say whether it needs an invitation. If it does, Hisar tells you when you create the account.</p>
            <label v-if="showsInviteField && !isChecking">
              <span>{{ requiresInvitation ? 'Invite link or code' : 'Invite link or code (if you have one)' }}</span>
              <input v-model="inviteCode" name="inviteCode" placeholder="Invite link or code" autocomplete="off" spellcheck="false" :required="requiresInvitation" :aria-label="requiresInvitation ? 'Invite link or code, required' : 'Invite link or code (if you have one)'" :aria-invalid="!!errors.inviteCode" aria-describedby="invite-error" @change="acceptInvitation()" @paste="pasteInvitation" />
            </label>
            <p v-if="errors.inviteCode" id="invite-error" class="error" role="alert">{{ errors.inviteCode }}</p>
          </template>
          <template v-if="showsAccountFields">
            <p class="hint muted">All account fields are required.</p>
            <label><span>Name</span><input v-model="name" name="name" placeholder="Your name" autocomplete="name" aria-label="Name, required" required :aria-invalid="!!errors.name" aria-describedby="name-error" /></label>
            <p v-if="errors.name" id="name-error" class="error" role="alert">{{ errors.name }}</p>
            <div v-if="invitation?.email"><strong>Email</strong><p data-testid="invited-email">{{ email }}</p><p class="muted">This invitation is for this address.</p></div>
            <label v-else><span>Email</span><input v-model="email" name="email" type="email" autocomplete="username" aria-label="Email, required" required :aria-invalid="!!errors.email" aria-describedby="email-error" /></label>
            <p v-if="errors.email" id="email-error" class="error" role="alert">{{ errors.email }}</p>
            <label><span>Master password</span><CredentialField><input v-model="password" name="password" type="password" autocomplete="new-password" minlength="10" placeholder="At least 10 characters" aria-label="Master password, required" required aria-describedby="sign-up-password-feedback password-error" :aria-invalid="!!errors.password || passwordFeedback.isBlocking && !!password" /></CredentialField></label>
            <StrengthMeter :password="password" />
            <p id="sign-up-password-feedback" class="password-feedback" :class="passwordFeedback.isBlocking ? 'blocking' : 'muted'" data-testid="password-feedback">{{ passwordFeedback.message }}</p>
            <p v-if="errors.password" id="password-error" class="error" role="alert">{{ errors.password }}</p>
            <label><span>Repeat master password</span><CredentialField secret-name="repeated master password"><input v-model="repeatedPassword" name="repeatedPassword" type="password" autocomplete="new-password" aria-label="Repeat master password, required" required :aria-invalid="!!errors.repeatedPassword" aria-describedby="sign-up-password-mismatch" /></CredentialField></label>
            <p v-if="errors.repeatedPassword" id="sign-up-password-mismatch" class="error" role="alert">{{ errors.repeatedPassword }}</p>
            <p class="hint muted">Choose a master password you can remember. Hisar never sends it to the server, so nobody can reset it for you.</p>
            <p class="hint muted">Next, Hisar creates your Secret Key. You don't choose one.</p>
          </template>
          <button v-if="policy.state !== 'error' || invitation" class="primary full-width submit" type="submit" :disabled="store.isBusy || isChecking">{{ store.isBusy ? "Creating account…" : "Create account" }}</button>
          <p v-if="isHostedSignup && showsAccountFields" class="hint muted" data-testid="legal-links">By creating an account you agree to the <a href="https://eu.tryhisar.com/terms" target="_blank">Terms</a> and <a href="https://eu.tryhisar.com/privacy" target="_blank">Privacy Policy</a>.</p>
        </form>
        </template>
      </div>
    </main>
  </div>
  <SecretKeyHelp v-if="isHelpShown" @close="isHelpShown = false" />
</template>

<style scoped>
.invitation-card { margin: 12px 0; padding: 12px; border: 1px solid var(--color-border); border-radius: var(--radius); overflow-wrap: anywhere; }
.help { display: block; margin: 14px auto 0; }
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
