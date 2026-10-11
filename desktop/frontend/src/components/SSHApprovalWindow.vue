<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Events } from "@wailsio/runtime";
import { api } from "../api";
import {
  addApproval,
  approvalStore,
  authorizeApproval,
  authorizeWithPassword,
  choosePassword,
  denyApproval,
  failApproval,
  loadAllPendingApproval,
  removeApproval,
  setRemembered,
} from "../approval";
import { startThemeSync } from "../theme";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";

const allCompactAction: Record<string, string> = {
  push: "Push",
  pull: "Pull",
  fetch: "Fetch",
  clone: "Clone",
  "sign-in": "Sign in",
  "sign-commit": "Sign a commit",
  "sign-tag": "Sign a tag",
  "use-key": "Use SSH key",
};

const password = ref("");
const promptSlot = ref<HTMLElement | null>(null);
const passwordInput = ref<HTMLInputElement | null>(null);
const denyButton = ref<HTMLButtonElement | null>(null);
const contentElement = ref<HTMLElement | null>(null);
const approval = computed(() => approvalStore.allApproval[0]);
const isCommandLine = computed(() => approval.value?.kind === "command-line");
const isAgent = computed(() => approval.value?.kind === "agent");
const isForwarded = computed(() => approval.value?.isForwarded === true);
const isSigning = computed(() => approval.value?.purpose === "sign-commit" || approval.value?.purpose === "sign-tag");
const isCompactShown = computed(() => approval.value?.isCompact === true && !approvalStore.isExpanded);
const isPasswordShown = computed(() => approval.value?.isPasswordUnlock === true || approvalStore.isPasswordChosen);
const isTouchIDUsed = computed(() => approval.value?.isTouchIDApproval === true && !isPasswordShown.value);
const isWaitingForTouchID = computed(() => approvalStore.isAuthorizing && isTouchIDUsed.value);
const canChoosePassword = computed(() => approval.value?.isTouchIDApproval === true && !isPasswordShown.value);

const isAppRequester = computed(() => !approval.value?.requesterKind || approval.value.requesterKind === "app");

// inlineAppName is the app inside a sentence: "iTerm2", or "a background process".
const inlineAppName = computed(() => {
  const appName = approval.value?.appName ?? "";
  if (isAppRequester.value || /^[A-Z]{2}/.test(appName)) {
    return appName;
  }
  return appName.charAt(0).toLowerCase() + appName.slice(1);
});

const requesterName = computed(() => {
  const current = approval.value;
  if (!current) {
    return "";
  }
  if (current.isForwarded) {
    return current.forwardedFromHost || "Another computer";
  }
  if (current.kind === "agent") {
    return current.agentName;
  }
  if (current.kind === "command-line" && current.appName !== current.clientName) {
    return `hisar in ${inlineAppName.value}`;
  }
  return current.appName;
});

const allChainProcess = computed(() => (approval.value?.isForwarded ? [] : (approval.value?.allChainProcess ?? [])));
const chainText = computed(() => allChainProcess.value.map((process) => process.name).join(" ← "));
const chainTitle = computed(() => allChainProcess.value.map((process) => process.commandLine || process.name).join("\n"));
const appSigner = computed(() => (approval.value?.isForwarded || !isAppRequester.value ? "" : (approval.value?.appSigner ?? "")));
const appWarning = computed(() => (approval.value?.isForwarded ? "" : (approval.value?.appWarning ?? "")));

const purposeText = computed(() => {
  const current = approval.value;
  if (!current) {
    return "";
  }
  if (current.kind === "command-line" || current.kind === "agent") {
    return current.action || "use Hisar";
  }
  return [current.action || "use your SSH key", current.target].filter(Boolean).join(" ");
});

const subtitle = computed(() => {
  const current = approval.value;
  if (!current) {
    return "";
  }
  if (current.kind === "command-line") {
    return current.terminalName ? `from the terminal session ${current.terminalName}` : "from the command line";
  }
  if (current.kind === "agent") {
    return current.appName ? `via ${current.clientName} in ${inlineAppName.value}` : `via ${current.clientName}`;
  }
  return current.keyTitle ? `with your SSH key “${current.keyTitle}”` : "Unlock Hisar to use your SSH keys";
});

const allDetailLine = computed(() => {
  const current = approval.value;
  if (!current) {
    return [];
  }
  if (current.kind === "command-line") {
    const program = current.action ? current.commandLine : current.clientPath || current.clientName;
    return [program, current.terminalName ? `/dev/${current.terminalName}` : ""].filter(Boolean);
  }
  if (current.kind === "agent") {
    if (current.purpose === "connect") {
      return [current.clientPath || current.clientName, current.terminalName ? `/dev/${current.terminalName}` : ""].filter(Boolean);
    }
    return [current.commandLine, current.workingDirectory].filter(Boolean);
  }
  if (isSigning.value) {
    const firstLine = current.commitSubject ? `“${current.commitSubject}”` : current.commandLine || "a commit";
    return [firstLine, [current.repositoryPath, current.branch].filter(Boolean).join(" · ")].filter(Boolean);
  }
  const location = [current.repositoryPath, current.hostAndPort].filter(Boolean).join(" · ");
  return [current.commandLine || current.clientPath || current.clientName, location].filter(Boolean);
});

const allSecretName = computed(() => approval.value?.allSecretName ?? []);
const rememberLabel = computed(
  () => approval.value?.rememberLabel || `Don’t ask again for ${isAppRequester.value ? approval.value?.appName : approval.value?.clientName} for 1 hour`,
);

const unlockNote = computed(() => {
  if (isAgent.value) {
    return "Hisar is locked; unlocking it also approves this request.";
  }
  return isCommandLine.value ? "Hisar is locked; unlocking it also approves this terminal." : "Hisar is locked; approving also unlocks it.";
});

const agentNote = computed(() => {
  const current = approval.value;
  if (!current || current.kind !== "agent") {
    return "";
  }
  if (current.purpose === "connect") {
    return `Lets ${current.agentName} list and search the vaults available to agents until 10 minutes after its last request, or until Hisar locks. Using a secret always asks.`;
  }
  return current.isConnectionApproval ? `Approving also lets ${current.agentName} list and search the vaults available to agents.` : "";
});

const compactSubtitle = computed(() => {
  const current = approval.value;
  if (!current) {
    return "";
  }
  const place = isSigning.value ? current.repositoryPath || "a commit" : current.target || current.hostAndPort || "SSH";
  return current.keyTitle ? `${place} with “${current.keyTitle}”` : place;
});

const touchIDLabel = computed(() => (approval.value?.isUnlockRequired ? "Touch ID to unlock and approve" : "Touch ID to approve"));
const passwordButtonLabel = computed(() => (approval.value?.isUnlockRequired ? "Unlock" : "Approve"));
const approveButtonLabel = computed(() => {
  if (isForwarded.value) {
    return "Allow once";
  }
  return approval.value?.isUnlockRequired ? "Unlock" : "Approve";
});

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

// fitWindow sizes the window to the dialog or the compact card.
async function fitWindow(): Promise<void> {
  await nextTick();
  const content = contentElement.value;
  if (!content || !approval.value) {
    return;
  }
  await api.fitSSHApprovalWindow(isCompactShown.value, Math.ceil(content.getBoundingClientRect().height)).catch(() => undefined);
  if (isWaitingForTouchID.value) {
    await sendPromptFrame();
  }
}

async function startAuthorizing(): Promise<void> {
  const current = approval.value;
  if (!current || approvalStore.isAuthorizing) {
    return;
  }
  if (isTouchIDUsed.value) {
    approvalStore.isAuthorizing = true;
    await sendPromptFrame();
  }
  await authorizeApproval(current.requestID);
}

async function submitPassword(): Promise<void> {
  const current = approval.value;
  if (!current || approvalStore.isAuthorizing) {
    return;
  }
  if (!password.value) {
    passwordInput.value?.focus();
    return;
  }
  const typedPassword = password.value;
  password.value = "";
  await authorizeWithPassword(current.requestID, typedPassword);
}

async function usePassword(): Promise<void> {
  await choosePassword();
  approvalStore.isExpanded = true;
  await nextTick();
  passwordInput.value?.focus();
}

function deny(): void {
  if (approval.value) {
    denyApproval(approval.value.requestID);
  }
}

function expandCompact(): void {
  approvalStore.isExpanded = true;
}

watch(
  () => approval.value?.requestID,
  async (requestID) => {
    password.value = "";
    if (!requestID) {
      api.clearApprovalTouchIDFrame().catch(() => undefined);
      return;
    }
    await nextTick();
    if (approval.value?.isPasswordUnlock) {
      passwordInput.value?.focus();
    } else if (isForwarded.value) {
      denyButton.value?.focus();
    } else if (approval.value?.isTouchIDApproval) {
      startAuthorizing();
    }
  },
);

watch(
  () => [approval.value?.requestID, isCompactShown.value, isPasswordShown.value, approvalStore.failureMessage, approvalStore.allApproval.length, isWaitingForTouchID.value],
  () => fitWindow(),
);

function handleKey(event: KeyboardEvent): void {
  if (!approval.value) {
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    deny();
    return;
  }
  // Focused buttons and the password form answer Return themselves.
  if (event.key !== "Enter" || event.target instanceof HTMLButtonElement || event.target instanceof HTMLInputElement) {
    return;
  }
  event.preventDefault();
  if (isPasswordShown.value) {
    submitPassword();
  } else if (isForwarded.value) {
    deny();
  } else if (!approvalStore.isAuthorizing) {
    startAuthorizing();
  }
}

onMounted(() => {
  allRemoveListener = [
    startThemeSync(),
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
  <main class="approval-window" :class="{ 'is-compact': isCompactShown }">
    <div ref="contentElement" class="approval-content">
      <template v-if="approval">
        <section
          v-if="isCompactShown"
          class="compact-card"
          role="alertdialog"
          aria-labelledby="approval-title"
          :title="chainText ? `Show details\n${chainText}` : 'Show details'"
          data-testid="ssh-approval"
          @click="expandCompact"
        >
          <div class="requester-icon small" aria-hidden="true">
            <img v-if="approval.appIconURL" :src="approval.appIconURL" alt="" data-testid="app-icon" />
            <span v-else class="icon-fallback"><AppIcon name="terminal" :size="20" /></span>
            <span class="hisar-badge"><HisarLogo :size="13" /></span>
          </div>
          <div class="compact-body">
            <p id="approval-title" class="compact-title" data-testid="approval-compact-title">
              <strong data-testid="approval-app">{{ approval.appName }}</strong> · {{ allCompactAction[approval.purpose] ?? "Use SSH key" }}
            </p>
            <p class="compact-subtitle">{{ compactSubtitle }}</p>
            <p v-if="appWarning" class="compact-subtitle app-warning-line" data-testid="approval-app-warning">
              <AppIcon name="warning" :size="12" /> {{ approval.appSigner }} — check this is the app you expect
            </p>
            <p v-if="approval.isPhoneApproval" class="compact-subtitle phone-note" data-testid="approval-phone">or approve on your iPhone</p>
            <p v-if="approvalStore.failureMessage" class="failure" role="alert">{{ approvalStore.failureMessage }}</p>
            <div class="compact-actions">
              <div v-if="isWaitingForTouchID" class="compact-touch-id" data-testid="approval-touch-id">
                <div ref="promptSlot" class="prompt-slot small">
                  <AppIcon v-if="!api.hasNativeTouchIDView" name="fingerprint" :size="20" />
                </div>
                <span>{{ isSigning ? "Touch ID to sign" : "Touch ID to allow for 1 hour" }}</span>
              </div>
              <button v-else class="link" data-testid="authorize-approval" @click.stop="startAuthorizing">Try Touch ID again</button>
              <button class="compact-deny" data-testid="deny-approval" @click.stop="deny">Deny</button>
            </div>
          </div>
        </section>

        <section v-else class="dialog" role="alertdialog" aria-labelledby="approval-title" aria-describedby="approval-subtitle" data-testid="ssh-approval">
          <div class="drag-area"></div>
          <div v-if="isForwarded" class="forwarded-banner" role="note" data-testid="approval-forwarded">
            <AppIcon name="warning" :size="16" />
            <p>
              <strong>Forwarded from another computer</strong> — this request comes from
              <strong class="monospace">{{ approval.forwardedFromHost || "a remote host" }}</strong> through agent forwarding, not from this Mac
            </p>
          </div>

          <header class="request-header">
            <div class="requester-icon" aria-hidden="true">
              <img v-if="approval.appIconURL && !isForwarded" :src="approval.appIconURL" alt="" data-testid="app-icon" />
              <span v-else class="icon-fallback"><AppIcon :name="isForwarded ? 'globe' : 'terminal'" :size="28" /></span>
              <span class="hisar-badge"><HisarLogo :size="18" /></span>
            </div>
            <div class="request-text">
              <span v-if="isSigning" class="signing-badge" data-testid="approval-signing-badge">
                <AppIcon name="edit" :size="12" /> {{ approval.purpose === "sign-tag" ? "Sign a tag" : "Sign a commit" }}
              </span>
              <h1 id="approval-title">
                <span data-testid="approval-app">{{ requesterName }}</span> wants to {{ purposeText }}
              </h1>
              <p id="approval-subtitle" class="subtitle" :title="approval.fingerprint">{{ subtitle }}</p>
              <p v-if="chainText || appSigner" class="requester-line" data-testid="approval-requester">
                <span v-if="chainText" class="chain" :title="chainTitle" data-testid="approval-chain">{{ chainText }}</span>
                <span v-if="appSigner" class="signer" :class="{ 'is-unverified': appWarning }" :title="approval.appPath" data-testid="approval-signer">
                  <AppIcon :name="appWarning ? 'warning' : 'shield'" :size="11" /> {{ appSigner }}
                </span>
              </p>
            </div>
          </header>

          <div v-if="appWarning" class="app-warning" role="note" data-testid="approval-app-warning">
            <AppIcon name="warning" :size="15" />
            <p>{{ appWarning }}</p>
          </div>

          <div v-if="allDetailLine.length > 0" class="detail-box" data-testid="approval-details">
            <code v-for="(line, index) in allDetailLine" :key="index" class="detail-line" :class="{ secondary: index > 0 }" :title="line">{{ line }}</code>
          </div>

          <div v-if="(isCommandLine || isAgent) && allSecretName.length > 0" class="secret-list" data-testid="approval-secrets">
            <p class="secret-heading">{{ allSecretName.length + approval.moreSecretCount === 1 ? "Secret" : "Secrets" }}</p>
            <ul>
              <li v-for="(name, index) in allSecretName" :key="index" :title="name"><AppIcon name="key" :size="12" /> {{ name }}</li>
              <li v-if="approval.moreSecretCount > 0" class="muted">and {{ approval.moreSecretCount }} more</li>
            </ul>
          </div>

          <div v-if="approval.warning" class="reveal-warning" role="note" data-testid="approval-warning">
            <AppIcon name="eye" :size="15" />
            <p>{{ approval.warning }}</p>
          </div>

          <label v-if="approval.isRememberable" class="remember">
            <input
              type="checkbox"
              name="isRemembered"
              :checked="approvalStore.isRemembered"
              @change="setRemembered(approval.requestID, ($event.target as HTMLInputElement).checked)"
            />
            {{ rememberLabel }}
          </label>

          <p v-if="approval.isUnlockRequired" class="unlock-note" data-testid="approval-unlock-note">
            <AppIcon name="lock" :size="13" />
            {{ unlockNote }}
          </p>
          <p v-if="approvalStore.failureMessage" class="failure" role="alert">{{ approvalStore.failureMessage }}</p>

          <form v-if="isPasswordShown" class="password-row" @submit.prevent="submitPassword">
            <input
              ref="passwordInput"
              v-model="password"
              type="password"
              name="approvalPassword"
              aria-label="Master password"
              placeholder="Master password"
              autocomplete="current-password"
              data-testid="approval-password"
            />
            <button type="submit" class="primary" data-testid="submit-password" :disabled="approvalStore.isAuthorizing || !password">{{ passwordButtonLabel }}</button>
          </form>

          <p v-if="isCommandLine" class="muted note">Lasts until 10 minutes after this terminal’s last command, or until Hisar locks.</p>
          <p v-if="agentNote" class="muted note" data-testid="approval-agent-note">{{ agentNote }}</p>

          <footer class="footer-bar">
            <div v-if="isWaitingForTouchID" class="touch-id" data-testid="approval-touch-id">
              <div ref="promptSlot" class="prompt-slot">
                <AppIcon v-if="!api.hasNativeTouchIDView" name="fingerprint" :size="26" />
              </div>
              <div class="touch-id-text">
                <span>{{ touchIDLabel }}</span>
                <button class="link" data-testid="use-password" @click="usePassword">or use your master password</button>
                <span v-if="approval.isPhoneApproval" class="phone-note" data-testid="approval-phone">or approve on your iPhone</span>
              </div>
            </div>
            <div v-else-if="!isPasswordShown" class="touch-id manual">
              <button
                :class="{ primary: !isForwarded }"
                data-testid="authorize-approval"
                :disabled="approvalStore.isAuthorizing"
                @click="startAuthorizing"
              >
                <AppIcon v-if="isTouchIDUsed" name="fingerprint" :size="16" />
                {{ approvalStore.failureMessage && !isForwarded ? "Try again" : approveButtonLabel }}
              </button>
              <button v-if="canChoosePassword" class="link" data-testid="use-password" @click="usePassword">or use your master password</button>
              <span v-if="approval.isPhoneApproval" class="phone-note" data-testid="approval-phone">or approve on your iPhone</span>
            </div>
            <span v-else-if="approval.isPhoneApproval" class="phone-note footer-spacer" data-testid="approval-phone">or approve on your iPhone</span>
            <span v-else class="footer-spacer"></span>
            <span v-if="approvalStore.allApproval.length > 1" class="muted waiting">{{ approvalStore.allApproval.length - 1 }} more waiting</span>
            <button ref="denyButton" :class="{ primary: isForwarded }" data-testid="deny-approval" @click="deny">Deny</button>
          </footer>
        </section>
      </template>
      <p v-else class="muted no-request">No requests waiting.</p>
    </div>
  </main>
</template>

<style scoped>
.approval-window {
  height: 100vh;
  overflow: hidden;
  background: var(--color-panel);
}

.approval-window.is-compact {
  background: var(--color-surface);
}

.approval-content {
  display: flex;
  flex-direction: column;
}

.drag-area {
  height: 30px;
  flex-shrink: 0;
  --wails-draggable: drag;
}

.dialog {
  display: flex;
  flex-direction: column;
  padding: 0 28px;
}

.forwarded-banner {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 0 0 16px;
  padding: 10px 12px;
  border: 1px solid var(--color-danger-border);
  border-radius: var(--radius);
  background: var(--color-danger-background);
  color: var(--color-danger-text);
  font-size: 12px;
  line-height: 1.45;
}

.forwarded-banner .icon {
  margin-top: 1px;
  color: var(--color-danger);
}

.forwarded-banner p {
  margin: 0;
}

.forwarded-banner strong {
  color: inherit;
}

.request-header {
  display: flex;
  align-items: flex-start;
  gap: 16px;
}

.requester-icon {
  position: relative;
  flex: none;
  width: 64px;
  height: 64px;
}

.requester-icon img {
  width: 64px;
  height: 64px;
  object-fit: contain;
}

.icon-fallback {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  border: 1px solid var(--color-border-strong);
  border-radius: 13px;
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.hisar-badge {
  position: absolute;
  right: -4px;
  bottom: -4px;
  display: inline-flex;
  padding: 2px;
  border-radius: 7px;
  background: var(--color-panel);
}

.requester-icon.small {
  width: 36px;
  height: 36px;
}

.requester-icon.small img {
  width: 36px;
  height: 36px;
}

.requester-icon.small .icon-fallback {
  border-radius: 9px;
}

.requester-icon.small .hisar-badge {
  right: -3px;
  bottom: -3px;
  padding: 1.5px;
  border-radius: 5px;
  background: var(--color-surface);
}

.request-text {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  min-width: 0;
  padding-top: 2px;
}

.signing-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 6px;
  padding: 2px 8px 2px 7px;
  border: 1px solid var(--color-warning-border);
  border-radius: 999px;
  background: var(--color-warning-background);
  color: var(--color-warning);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

h1 {
  margin: 0;
  font-size: 17px;
  font-weight: 650;
  line-height: 1.3;
  overflow-wrap: anywhere;
}

.subtitle {
  margin: 4px 0 0;
  color: var(--color-text-secondary);
  font-size: 13px;
  overflow-wrap: anywhere;
}

.requester-line {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 3px;
  margin: 8px 0 0;
  font-size: 11px;
  color: var(--color-text-muted);
}

.chain {
  font-family: "SF Mono", ui-monospace, Menlo, monospace;
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}

.signer {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.signer.is-unverified,
.compact-subtitle.app-warning-line {
  color: var(--color-danger);
}

.app-warning {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-top: 16px;
  padding: 10px 12px;
  border: 1px solid var(--color-danger-border);
  border-radius: var(--radius);
  background: var(--color-danger-background);
  color: var(--color-danger-text);
  font-size: 12px;
  line-height: 1.45;
}

.app-warning .icon {
  flex: none;
  margin-top: 1px;
  color: var(--color-danger);
}

.app-warning p {
  margin: 0;
}

.detail-box {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 22px;
  padding: 12px 14px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-background);
}

.detail-line {
  overflow: hidden;
  font-family: "SF Mono", ui-monospace, Menlo, monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: var(--color-text);
  user-select: text;
  -webkit-user-select: text;
}

.detail-line.secondary {
  color: var(--color-text-muted);
}

.secret-list {
  margin-top: 14px;
}

.secret-heading {
  margin: 0 0 6px 2px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.secret-list ul {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.secret-list li {
  display: flex;
  align-items: center;
  gap: 7px;
  overflow: hidden;
  font-size: 13px;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.reveal-warning {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin-top: 14px;
  padding: 10px 12px;
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius);
  background: var(--color-warning-background);
  color: var(--color-warning);
  font-size: 12px;
  line-height: 1.45;
}

.reveal-warning .icon {
  flex: none;
  margin-top: 1px;
}

.reveal-warning p {
  margin: 0;
}

.remember {
  display: flex;
  align-items: center;
  gap: 9px;
  margin: 20px 0 0 2px;
  font-size: 13px;
  font-weight: 400;
  color: var(--color-text);
}

.remember input {
  width: auto;
  margin: 0;
}

.unlock-note,
.failure {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.failure {
  color: var(--color-danger);
}

.password-row {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}

.password-row input {
  flex: 1;
}

.note {
  margin: 12px 0 0;
  font-size: 11px;
}

.footer-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 26px -28px 0;
  padding: 16px 28px 18px;
  border-top: 1px solid var(--color-border);
  background: var(--color-surface);
}

.touch-id {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-right: auto;
  min-width: 0;
}

.touch-id.manual {
  gap: 14px;
}

.touch-id-text {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 1px;
  font-size: 13px;
}

.touch-id-text .link,
.touch-id.manual .link {
  font-size: 12px;
  color: var(--color-text-muted);
}

.prompt-slot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: 40px;
  height: 40px;
  color: var(--color-danger);
}

.prompt-slot.small {
  width: 26px;
  height: 26px;
}

.footer-spacer {
  margin-right: auto;
}

.phone-note {
  font-size: 12px;
  color: var(--color-text-muted);
}

.waiting {
  font-size: 12px;
}

.compact-card {
  display: flex;
  gap: 12px;
  padding: 14px 14px 12px;
  cursor: pointer;
}

.compact-body {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.compact-title,
.compact-subtitle {
  margin: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.compact-title {
  font-size: 13px;
}

.compact-subtitle {
  margin-top: 1px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.compact-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
}

.compact-touch-id {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-right: auto;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.compact-actions .link {
  margin-right: auto;
  font-size: 12px;
}

.compact-deny {
  padding: 4px 12px;
  font-size: 12px;
}

.no-request {
  margin: 40px auto;
}
</style>
