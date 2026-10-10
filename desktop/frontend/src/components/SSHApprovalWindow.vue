<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Events } from "@wailsio/runtime";
import { api } from "../api";
import {
  addApproval,
  approvalStore,
  authorizeApproval,
  authorizeWithPassword,
  denyApproval,
  failApproval,
  loadAllPendingApproval,
  removeApproval,
  setForAllApplication,
} from "../approval";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";

const isDetailOpen = ref(false);
const password = ref("");
const promptSlot = ref<HTMLElement | null>(null);
const passwordInput = ref<HTMLInputElement | null>(null);
const approval = computed(() => approvalStore.allApproval[0]);
const destination = computed(() => approval.value?.destinationHost || approval.value?.destinationFingerprint || "");
const isProgramShownSeparately = computed(() => approval.value !== undefined && approval.value.clientName !== approval.value.appName);
const isCommandLine = computed(() => approval.value?.kind === "command-line");
const isPasswordUnlock = computed(() => isCommandLine.value && approval.value?.isPasswordUnlock === true);
const isTouchIDUsed = computed(() => approval.value?.isTouchIDApproval === true && !isPasswordUnlock.value);
const isWaitingForTouchID = computed(() => approvalStore.isAuthorizing && isTouchIDUsed.value);
const authorizeLabel = computed(() => (isCommandLine.value && approval.value?.isUnlockRequired ? "Unlock" : "Authorize"));
let allRemoveListener: (() => void)[] = [];

// sendPromptFrame places macOS's Touch ID view over the slot, so touching the sensor approves without a click.
async function sendPromptFrame(): Promise<void> {
  await nextTick();
  const slot = promptSlot.value;
  if (!slot) {
    return;
  }
  const rectangle = slot.getBoundingClientRect();
  await api.setApprovalTouchIDFrame(rectangle.x, rectangle.y, rectangle.width, rectangle.height).catch(() => undefined);
}

async function startAuthorizing(): Promise<void> {
  const current = approval.value;
  if (!current || approvalStore.isAuthorizing) {
    return;
  }
  if (isPasswordUnlock.value) {
    if (!password.value) {
      passwordInput.value?.focus();
      return;
    }
    const typedPassword = password.value;
    password.value = "";
    await authorizeWithPassword(current.requestID, typedPassword);
    return;
  }
  if (current.isTouchIDApproval) {
    approvalStore.isAuthorizing = true;
    await sendPromptFrame();
  }
  await authorizeApproval(current.requestID);
}

watch(
  () => approval.value?.requestID,
  (requestID) => {
    isDetailOpen.value = false;
    password.value = "";
    if (!requestID) {
      api.clearApprovalTouchIDFrame().catch(() => undefined);
    } else if (isPasswordUnlock.value) {
      nextTick(() => passwordInput.value?.focus());
    } else if (approval.value?.isTouchIDApproval) {
      startAuthorizing();
    }
  },
);

watch(isDetailOpen, () => {
  if (isWaitingForTouchID.value) {
    sendPromptFrame();
  }
});

function handleKey(event: KeyboardEvent): void {
  if (!approval.value) {
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    denyApproval(approval.value.requestID);
  } else if (event.key === "Enter" && !approvalStore.isAuthorizing && !(event.target instanceof HTMLButtonElement)) {
    event.preventDefault();
    startAuthorizing();
  }
}

onMounted(() => {
  allRemoveListener = [
    Events.On("ssh-approval", (event) => addApproval(event.data)),
    Events.On("ssh-approval-closed", (event) => removeApproval(event.data)),
    Events.On("ssh-approval-failed", (event) => failApproval(event.data.requestID, event.data.message)),
  ];
  loadAllPendingApproval();
  window.addEventListener("keydown", handleKey);
  window.addEventListener("resize", sendPromptFrame);
});

onBeforeUnmount(() => {
  allRemoveListener.forEach((removeListener) => removeListener());
  window.removeEventListener("keydown", handleKey);
  window.removeEventListener("resize", sendPromptFrame);
  api.clearApprovalTouchIDFrame().catch(() => undefined);
});
</script>

<template>
  <main class="approval-window">
    <div class="title-bar"></div>
    <section v-if="approval" class="approval" role="alertdialog" aria-labelledby="approval-title" data-testid="ssh-approval">
      <h1>Hisar Access Requested</h1>

      <div class="parties" aria-hidden="true">
        <img v-if="approval.appIconURL" class="app-icon" :src="approval.appIconURL" alt="" data-testid="app-icon" />
        <span v-else class="app-icon fallback"><AppIcon name="terminal" :size="30" /></span>
        <span class="connector"><span class="connector-check"><AppIcon name="check" :size="11" /></span></span>
        <HisarLogo :size="56" />
      </div>

      <p v-if="isCommandLine" id="approval-title" class="question">
        <strong class="monospace">{{ approval.clientName }}</strong>
        <template v-if="isProgramShownSeparately"> in <strong data-testid="approval-app">{{ approval.appName }}</strong></template>
        wants to use Hisar
      </p>
      <p v-else id="approval-title" class="question">
        Allow <strong data-testid="approval-app">{{ approval.appName }}</strong> to use SSH key
      </p>

      <div v-if="isCommandLine" class="key-card" :class="{ open: isDetailOpen }">
        <button class="key-row" :aria-expanded="isDetailOpen" data-testid="approval-key" @click="isDetailOpen = !isDetailOpen">
          <span class="chevron"><AppIcon name="chevronRight" :size="14" /></span>
          <span class="key-icon terminal"><AppIcon name="terminal" :size="18" /></span>
          <strong>{{ approval.terminalName ? `Terminal session ${approval.terminalName}` : "Command-line tool" }}</strong>
        </button>
        <dl v-if="isDetailOpen" class="details" data-testid="approval-details">
          <div>
            <dt>Program</dt>
            <dd class="monospace">{{ approval.clientPath || approval.clientName }}</dd>
          </div>
          <div v-if="approval.terminalName">
            <dt>Terminal</dt>
            <dd class="monospace">/dev/{{ approval.terminalName }}</dd>
          </div>
        </dl>
      </div>

      <div v-else class="key-card" :class="{ open: isDetailOpen }">
        <button class="key-row" :aria-expanded="isDetailOpen" data-testid="approval-key" @click="isDetailOpen = !isDetailOpen">
          <span class="chevron"><AppIcon name="chevronRight" :size="14" /></span>
          <span class="key-icon"><AppIcon name="key" :size="18" /></span>
          <strong>{{ approval.keyTitle }}</strong>
        </button>
        <dl v-if="isDetailOpen" class="details" data-testid="approval-details">
          <div v-if="destination">
            <dt>Destination</dt>
            <dd :class="{ monospace: !approval.destinationHost }" data-testid="approval-destination">{{ destination }}</dd>
          </div>
          <div v-if="isProgramShownSeparately || approval.clientPath">
            <dt>Program</dt>
            <dd class="monospace">{{ approval.clientPath || approval.clientName }}</dd>
          </div>
          <div>
            <dt>Key</dt>
            <dd class="monospace">{{ approval.fingerprint }}</dd>
          </div>
        </dl>
      </div>

      <p v-if="approval.isForwarded" class="warning" data-testid="approval-forwarded">
        <AppIcon name="warning" :size="14" /> This request comes through a forwarded agent on another machine.
      </p>

      <template v-if="isCommandLine && approval.isUnlockRequired">
        <p class="unlock-note" data-testid="approval-unlock-note">
          <AppIcon name="lock" :size="13" /> Hisar is locked; unlocking it also approves this terminal.
        </p>
        <input
          v-if="isPasswordUnlock"
          ref="passwordInput"
          v-model="password"
          class="unlock-password"
          type="password"
          name="approvalPassword"
          aria-label="Master password"
          placeholder="Master password"
          autocomplete="current-password"
          data-testid="approval-password"
        />
      </template>

      <label v-if="!isCommandLine" class="all-applications">
        <input
          type="checkbox"
          name="isForAllApplication"
          :checked="approvalStore.isForAllApplication"
          @change="setForAllApplication(approval.requestID, ($event.target as HTMLInputElement).checked)"
        />
        Approve for all applications
      </label>

      <p v-if="approvalStore.failureMessage" class="failure" role="alert">{{ approvalStore.failureMessage }}</p>

      <div class="actions">
        <div v-if="isWaitingForTouchID" class="touch-id" data-testid="approval-touch-id">
          <div ref="promptSlot" class="prompt-slot">
            <AppIcon v-if="!api.hasNativeTouchIDView" name="fingerprint" :size="30" />
          </div>
          <span>{{ isCommandLine && approval.isUnlockRequired ? "Touch ID to unlock" : "Touch ID to approve" }}</span>
        </div>
        <span v-else-if="approvalStore.allApproval.length > 1" class="muted waiting">{{ approvalStore.allApproval.length - 1 }} more waiting</span>
        <button data-testid="deny-approval" @click="denyApproval(approval.requestID)">Deny</button>
        <button
          v-if="!isWaitingForTouchID"
          class="primary"
          data-testid="authorize-approval"
          :disabled="approvalStore.isAuthorizing || (isPasswordUnlock && !password)"
          @click="startAuthorizing"
        >
          <AppIcon v-if="isTouchIDUsed" name="fingerprint" :size="16" />
          {{ authorizeLabel }}
        </button>
      </div>
      <p v-if="isCommandLine" class="muted note">Lasts until 10 minutes after this terminal's last command, or until Hisar locks.</p>
      <p v-else class="muted note">Approvals last for an hour, or until Hisar locks.</p>
    </section>
    <p v-else class="muted idle">No requests waiting.</p>
  </main>
</template>

<style scoped>
.approval-window {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
  background: var(--color-panel);
}

.title-bar {
  height: 28px;
  flex-shrink: 0;
  --wails-draggable: drag;
}

/* The window has a fixed size: expanded details scroll, and Deny and Authorize stay in view. */
.approval {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  padding: 0 26px 18px;
  overflow-y: auto;
}

h1 {
  margin: 0 0 18px;
  font-size: 18px;
  font-weight: 650;
  text-align: center;
}

.parties {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0;
  margin-bottom: 16px;
}

.app-icon {
  width: 60px;
  height: 60px;
  object-fit: contain;
}

.app-icon.fallback {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border: 1px solid var(--color-border-strong);
  border-radius: 14px;
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.connector {
  position: relative;
  width: 72px;
  height: 2px;
  margin: 0 8px;
  background: var(--color-border-strong);
}

.connector-check {
  position: absolute;
  top: 50%;
  left: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--color-success);
  color: #0b0c0e;
  transform: translate(-50%, -50%);
}

.question {
  margin: 0 0 16px;
  font-size: 14px;
  text-align: center;
}

.key-card {
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.key-row {
  display: flex;
  align-items: center;
  gap: 10px;
  justify-content: flex-start;
  width: 100%;
  padding: 12px 14px;
  border: 0;
  background: transparent;
  text-align: left;
}

.chevron {
  display: inline-flex;
  color: var(--color-text-muted);
  transition: transform 120ms ease;
}

.key-card.open .chevron {
  transform: rotate(90deg);
}

.key-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: var(--color-surface-raised);
  color: var(--color-warning);
}

.details {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0 14px 12px 54px;
}

.details div {
  display: grid;
  grid-template-columns: 80px 1fr;
  gap: 8px;
}

.details dt {
  font-size: 12px;
  color: var(--color-text-muted);
}

.details dd {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: 12px;
  user-select: text;
  -webkit-user-select: text;
}

.key-icon.terminal {
  color: var(--color-text-secondary);
}

.unlock-note {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 14px 0 0;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.unlock-password {
  margin-top: 8px;
}

.warning,
.failure {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--color-warning);
}

.failure {
  color: var(--color-danger);
}

.all-applications {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 16px 0 0 4px;
  font-size: 13px;
  font-weight: 400;
}

.all-applications input {
  width: auto;
}

.actions {
  position: sticky;
  bottom: -18px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
  padding: 10px 0 18px;
  background: var(--color-panel);
}

.actions .primary {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.waiting {
  margin-right: auto;
  font-size: 12px;
}

.touch-id {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-right: auto;
  font-size: 13px;
  color: var(--color-text-secondary);
}

.prompt-slot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  color: var(--color-text-secondary);
}

.note {
  margin: 12px 0 0;
  font-size: 11px;
  text-align: right;
}

.idle {
  margin: auto;
}
</style>
