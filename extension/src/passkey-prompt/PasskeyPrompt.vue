<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { VisibilityGuard } from "../shared/click-guard";
import type { PasskeyChoice, PasskeyPromptCall, PasskeyPromptState } from "../shared/passkey";
import type { PromptChannel } from "./api";

const props = withDefaults(
  defineProps<{
    channel: PromptChannel;
    isVisibilityTracked?: boolean;
    pollIntervalMilliseconds?: number;
    pollLimit?: number;
    now?: () => number;
    isEventTrusted?: (event: Event) => boolean;
  }>(),
  {
    isVisibilityTracked: false,
    pollIntervalMilliseconds: 1000,
    pollLimit: 120,
    now: () => performance.now(),
    isEventTrusted: (event: Event) => event.isTrusted,
  },
);

const newLoginKey = "new";
const promptState = ref<PasskeyPromptState>();
const selectedLoginKey = ref(newLoginKey);
const errorMessage = ref("");
const isBusy = ref(false);
const isWaitingForUnlock = ref(false);
const root = ref<HTMLElement>();
const guard = new VisibilityGuard();
let observer: IntersectionObserver | undefined;
let pollTimer: ReturnType<typeof setTimeout> | undefined;
let lastReportedHeight = 0;

const heading = computed(() => {
  const state = promptState.value;
  if (!state) return "Hisar";
  if (state.view === "locked") return "Hisar is locked";
  if (state.kind === "create") return state.view === "excluded" ? "Passkey already saved" : "Save passkey";
  return state.view === "no-passkeys" ? "No passkey in Hisar" : "Sign in with a passkey";
});

async function call<Result>(promptCall: PasskeyPromptCall): Promise<Result> {
  return props.channel.call<Result>(promptCall);
}

async function loadState(): Promise<void> {
  try {
    promptState.value = await call<PasskeyPromptState>({ method: "getState" });
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error);
  }
}

function stopPolling(): void {
  if (pollTimer !== undefined) clearTimeout(pollTimer);
  pollTimer = undefined;
  isWaitingForUnlock.value = false;
}

function pollUntilUnlocked(remaining: number): void {
  pollTimer = setTimeout(async () => {
    await loadState();
    if (promptState.value?.view !== "locked" || remaining <= 1) {
      stopPolling();
      return;
    }
    pollUntilUnlocked(remaining - 1);
  }, props.pollIntervalMilliseconds);
}

function isClickAllowed(event: Event, isVisibilityRequired: boolean): boolean {
  if (!props.isEventTrusted(event)) return false;
  return !isVisibilityRequired || !props.isVisibilityTracked || guard.isInteractionAllowed(props.now());
}

async function run(promptCall: PasskeyPromptCall): Promise<void> {
  errorMessage.value = "";
  isBusy.value = true;
  try {
    await call(promptCall);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error);
    await loadState();
  } finally {
    isBusy.value = false;
  }
}

function choosePasskey(choice: PasskeyChoice, event: Event): void {
  if (!isClickAllowed(event, true) || isBusy.value) return;
  void run({ method: "choosePasskey", credentialId: choice.credentialId, vaultID: choice.vaultID, itemID: choice.itemID });
}

function savePasskey(event: Event): void {
  if (!isClickAllowed(event, true) || isBusy.value) return;
  const login = promptState.value?.allLogin.find((candidate) => `${candidate.vaultID}/${candidate.itemID}` === selectedLoginKey.value);
  void run(login ? { method: "savePasskey", vaultID: login.vaultID, itemID: login.itemID } : { method: "savePasskey" });
}

function runSimple(method: "confirmExcluded" | "useOtherDevice" | "dismiss", event: Event): void {
  if (!isClickAllowed(event, false)) return;
  void run({ method });
}

async function unlock(event: Event): Promise<void> {
  if (!isClickAllowed(event, false) || isWaitingForUnlock.value) return;
  errorMessage.value = "";
  isWaitingForUnlock.value = true;
  try {
    await call({ method: "unlock" });
    pollUntilUnlocked(props.pollLimit);
  } catch (error) {
    isWaitingForUnlock.value = false;
    errorMessage.value = error instanceof Error ? error.message : String(error);
  }
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape" && props.isEventTrusted(event)) void call({ method: "dismiss" }).catch(() => undefined);
}

async function reportHeight(): Promise<void> {
  await nextTick();
  const height = Math.ceil(root.value?.getBoundingClientRect().height ?? 0);
  if (height <= 0 || height === lastReportedHeight) return;
  lastReportedHeight = height;
  await call({ method: "resize", height }).catch(() => undefined);
}

function trackVisibility(): void {
  if (!props.isVisibilityTracked || !root.value) return;
  observer = new IntersectionObserver(
    (allEntry) => {
      for (const entry of allEntry) {
        const isVisible = (entry as IntersectionObserverEntry & { isVisible?: boolean }).isVisible === true;
        guard.recordVisibility(isVisible, props.now());
      }
    },
    { threshold: [1], trackVisibility: true, delay: 100 } as IntersectionObserverInit,
  );
  observer.observe(root.value);
}

watch([promptState, errorMessage, isWaitingForUnlock], reportHeight);
watch(
  () => promptState.value?.view,
  (view) => {
    const state = promptState.value;
    if (view !== "save" || !state) return;
    selectedLoginKey.value = newLoginKey;
    const sameUser = state.allLogin.find((login) => login.isPersonal === true && login.username.toLowerCase() === state.userName.toLowerCase() && state.userName !== "");
    if (sameUser) selectedLoginKey.value = `${sameUser.vaultID}/${sameUser.itemID}`;
  },
);

onMounted(async () => {
  window.addEventListener("keydown", handleKeydown);
  trackVisibility();
  await loadState();
});

onBeforeUnmount(() => {
  stopPolling();
  observer?.disconnect();
  window.removeEventListener("keydown", handleKeydown);
});
</script>

<template>
  <div ref="root" class="prompt" role="dialog" aria-label="Hisar passkey">
    <header>
      <svg class="logo" width="22" height="22" viewBox="0 0 32 32" aria-hidden="true">
        <rect width="32" height="32" rx="8" fill="#16181b" />
        <rect x="1" y="1" width="30" height="30" rx="7" fill="none" stroke="#30343a" stroke-width="2" />
        <path d="M8 24.5V10.5h3V7.5h2.5v3h5V7.5H21v3h3v14h-6v-5a2 2 0 0 0-4 0v5z" fill="#eceef1" />
      </svg>
      <div class="heading">
        <p class="title" data-testid="passkey-heading">{{ heading }}</p>
        <p v-if="promptState" class="subtitle" data-testid="passkey-site">{{ promptState.relyingPartyID }}</p>
      </div>
      <button type="button" class="close" aria-label="Close" data-testid="passkey-close" @click="runSimple('dismiss', $event)">
        <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true"><path d="M2 2l8 8M10 2l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" /></svg>
      </button>
    </header>

    <p v-if="!promptState && !errorMessage" class="message muted">Loading…</p>

    <template v-else-if="promptState?.view === 'locked'">
      <p class="message muted">Unlock Hisar to use your passkeys.</p>
      <button type="button" class="primary" :disabled="isWaitingForUnlock" data-testid="unlock-hisar" @click="unlock($event)">
        {{ isWaitingForUnlock ? "Waiting for Hisar…" : "Unlock Hisar" }}
      </button>
    </template>

    <template v-else-if="promptState?.view === 'choose'">
      <button
        v-for="choice in promptState.allPasskey"
        :key="`${choice.vaultID}/${choice.itemID}/${choice.credentialId}`"
        type="button"
        class="item"
        :disabled="isBusy"
        data-testid="passkey-choice"
        @click="choosePasskey(choice, $event)"
      >
        <span class="avatar" aria-hidden="true">{{ (choice.title || "?").slice(0, 1).toUpperCase() }}</span>
        <span class="text">
          <span class="item-title">{{ choice.userName || choice.userDisplayName || "Passkey" }}</span>
          <span class="item-subtitle">{{ choice.title }}</span>
        </span>
        <svg class="passkey-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <circle cx="9" cy="8" r="4" /><path d="M2 21v-1a6 6 0 0 1 10-4.5M19 13a3 3 0 1 0-3 3v5l1.5 1 1.5-1.5-1-1 1-1-1-1" />
        </svg>
      </button>
    </template>

    <p v-else-if="promptState?.view === 'no-passkeys'" class="message muted" data-testid="passkey-none">
      Hisar has no passkey for {{ promptState.relyingPartyID }}.
    </p>

    <template v-else-if="promptState?.view === 'save'">
      <p class="message">
        <span class="muted">For</span> <strong data-testid="passkey-user-name">{{ promptState.userName || "this account" }}</strong>
      </p>
      <fieldset class="logins" :disabled="isBusy">
        <legend class="muted">Save in</legend>
        <label class="option" data-testid="passkey-login-option">
          <input v-model="selectedLoginKey" type="radio" name="login" :value="newLoginKey" />
          <span class="text">
            <span class="item-title">New login</span>
            <span class="item-subtitle">in your Personal vault</span>
          </span>
        </label>
        <label v-for="login in promptState.allLogin" :key="login.vaultID + login.itemID" class="option" data-testid="passkey-login-option">
          <input v-model="selectedLoginKey" type="radio" name="login" :value="`${login.vaultID}/${login.itemID}`" />
          <span class="text">
            <span class="item-title">{{ login.title }}</span>
            <span class="item-subtitle">{{ login.username || "No username" }} · {{ login.vaultName }}</span>
            <span v-if="login.isPersonal !== true" class="item-subtitle sharing-scope">Shared. Other vault members can sign in with this passkey.</span>
          </span>
        </label>
      </fieldset>
      <button type="button" class="primary" :disabled="isBusy" data-testid="save-passkey" @click="savePasskey($event)">
        {{ isBusy ? "Saving…" : "Save passkey" }}
      </button>
    </template>

    <template v-else-if="promptState?.view === 'excluded'">
      <p class="message muted" data-testid="passkey-excluded">Hisar already has a passkey for this account on {{ promptState.relyingPartyID }}.</p>
      <button type="button" class="primary" data-testid="confirm-excluded" @click="runSimple('confirmExcluded', $event)">OK</button>
    </template>

    <p v-if="errorMessage" class="message error" data-testid="passkey-error">{{ errorMessage }}</p>

    <button type="button" class="other-device" data-testid="use-other-device" @click="runSimple('useOtherDevice', $event)">
      Use another device or browser
    </button>
  </div>
</template>

<style>
html,
body {
  margin: 0;
  padding: 0;
  background: transparent;
  overflow: hidden;
}

body {
  font: 13px/1.45 -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  color: #e8eaed;
  color-scheme: dark;
  -webkit-font-smoothing: antialiased;
  user-select: none;
}

.prompt {
  display: flex;
  flex-direction: column;
  gap: 4px;
  box-sizing: border-box;
  padding: 10px;
  border: 1px solid #30343a;
  border-radius: 12px;
  background: #111214;
}

header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 2px 2px 8px;
  border-bottom: 1px solid #23262b;
  margin-bottom: 4px;
}

.logo {
  flex: none;
}

.heading {
  flex: 1;
  min-width: 0;
}

.heading p {
  margin: 0;
}

.title {
  font-weight: 600;
}

.subtitle,
.item-subtitle,
.muted {
  color: #7c838d;
  font-size: 12px;
}

.subtitle,
.item-title,
.item-subtitle {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #7c838d;
  cursor: pointer;
}

.close:hover {
  background: #262a30;
  color: #e8eaed;
}

.item,
.option {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  box-sizing: border-box;
  padding: 7px 8px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.item:hover:not(:disabled),
.option:hover {
  background: #262a30;
}

.item:focus-visible,
.primary:focus-visible,
.other-device:focus-visible {
  outline: 2px solid #8a95a5;
  outline-offset: -2px;
}

.avatar {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 1px solid #30343a;
  border-radius: 7px;
  background: #1c1f23;
  color: #b4b9c0;
  font-size: 12px;
  font-weight: 600;
}

.text {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.item-title {
  font-weight: 500;
}

.passkey-icon {
  flex: none;
  color: #7c838d;
}

.logins {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-inline-size: 0;
  margin: 0;
  padding: 0;
  border: none;
}

.sharing-scope {
  white-space: normal;
  overflow-wrap: anywhere;
}

.logins legend {
  padding: 0 8px 4px;
}

.option input {
  margin: 0;
  accent-color: #eceef1;
}

.primary {
  margin-top: 6px;
  padding: 8px;
  border: 1px solid #eceef1;
  border-radius: 7px;
  background: #eceef1;
  color: #0b0c0e;
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}

.primary:hover:not(:disabled) {
  background: #ffffff;
}

.primary:disabled {
  opacity: 0.6;
  cursor: default;
}

.other-device {
  margin-top: 4px;
  padding: 7px 8px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: #b4b9c0;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.other-device:hover {
  background: #1c1f23;
  color: #e8eaed;
}

.message {
  margin: 0;
  padding: 4px 8px;
}

.error {
  color: #f2a7a9;
  font-size: 12px;
}
</style>
