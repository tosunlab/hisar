<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Events } from "@wailsio/runtime";
import { addApproval, approvalStore, authorizeApproval, denyApproval, failApproval, loadAllPendingApproval, removeApproval } from "../approval";
import AppIcon from "./AppIcon.vue";
import HisarLogo from "./HisarLogo.vue";

const isDetailOpen = ref(false);
const approval = computed(() => approvalStore.allApproval[0]);
const destination = computed(() => approval.value?.destinationHost || approval.value?.destinationFingerprint || "");
const isProgramShownSeparately = computed(() => approval.value !== undefined && approval.value.clientName !== approval.value.appName);
let allRemoveListener: (() => void)[] = [];

watch(
  () => approval.value?.requestID,
  () => (isDetailOpen.value = false),
);

function handleKey(event: KeyboardEvent): void {
  if (!approval.value || approvalStore.isAuthorizing) {
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    denyApproval(approval.value.requestID);
  } else if (event.key === "Enter" && !(event.target instanceof HTMLButtonElement)) {
    event.preventDefault();
    authorizeApproval(approval.value.requestID);
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
});

onBeforeUnmount(() => {
  allRemoveListener.forEach((removeListener) => removeListener());
  window.removeEventListener("keydown", handleKey);
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

      <p id="approval-title" class="question">
        Allow <strong data-testid="approval-app">{{ approval.appName }}</strong> to use SSH key
      </p>

      <div class="key-card" :class="{ open: isDetailOpen }">
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

      <label class="all-applications">
        <input v-model="approvalStore.isForAllApplication" type="checkbox" name="isForAllApplication" :disabled="approvalStore.isAuthorizing" />
        Approve for all applications
      </label>

      <p v-if="approvalStore.failureMessage" class="failure" role="alert">{{ approvalStore.failureMessage }}</p>

      <div class="actions">
        <span v-if="approvalStore.allApproval.length > 1" class="muted waiting">{{ approvalStore.allApproval.length - 1 }} more waiting</span>
        <button data-testid="deny-approval" :disabled="approvalStore.isAuthorizing" @click="denyApproval(approval.requestID)">Deny</button>
        <button class="primary" data-testid="authorize-approval" :disabled="approvalStore.isAuthorizing" @click="authorizeApproval(approval.requestID)">
          <AppIcon v-if="approval.isTouchIDApproval" name="fingerprint" :size="16" />
          {{ approvalStore.isAuthorizing && approval.isTouchIDApproval ? "Waiting for Touch ID…" : "Authorize" }}
        </button>
      </div>
      <p class="muted note">Approvals last for an hour, or until Hisar locks.</p>
    </section>
    <p v-else class="muted idle">No requests waiting.</p>
  </main>
</template>

<style scoped>
.approval-window {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  background: var(--color-panel);
}

.title-bar {
  height: 28px;
  flex-shrink: 0;
  --wails-draggable: drag;
}

.approval {
  display: flex;
  flex-direction: column;
  padding: 0 26px 18px;
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
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 18px;
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

.note {
  margin: 12px 0 0;
  font-size: 11px;
  text-align: right;
}

.idle {
  margin: auto;
}
</style>
