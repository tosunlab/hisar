<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { auth, getErrorMessage } from "../api";
import type { SignUpResult } from "../bridge/bridge";
import AccountStep from "./setup/AccountStep.vue";
import ChecksStep from "./setup/ChecksStep.vue";
import EmergencyKitStep from "./setup/EmergencyKitStep.vue";
import InvitePolicyStep from "./setup/InvitePolicyStep.vue";
import { findStep, getSetup, invalidSetupLinkMessage, setupSteps, type SetupStep } from "./setup/setup";
import SignInView from "./SignInView.vue";

const emit = defineEmits<{ leave: [] }>();

// The setup invite sits in the URL fragment, so it never reaches the server's access log or a Referer header. Pasting a
// newer setup URL into this tab changes only the fragment, which does not reload the page.
function readInviteToken(): string {
  return window.location.hash.replace(/^#/, "").trim();
}

function updateInviteToken(): void {
  inviteToken.value = readInviteToken();
}

const inviteToken = ref(readInviteToken());
const step = ref<SetupStep | null>(null);
const signUpResult = ref<SignUpResult | null>(null);
const loadError = ref("");
const stepNumber = computed(() => setupSteps.findIndex((setupStep) => setupStep.step === step.value) + 1);

onMounted(async () => {
  window.addEventListener("hashchange", updateInviteToken);
  try {
    const firstStep = findStep((await getSetup()).state);
    if (firstStep === null) {
      emit("leave");
      return;
    }
    step.value = firstStep;
  } catch (error) {
    loadError.value = getErrorMessage(error);
  }
});

onUnmounted(() => window.removeEventListener("hashchange", updateInviteToken));

function showEmergencyKit(result: SignUpResult): void {
  window.history.replaceState(null, "", "/admin/setup");
  signUpResult.value = result;
  step.value = "emergencyKit";
}

// leaveEmergencyKit drops the Secret Key and zeroes the kit's bytes once the admin saved them; the downloaded file and
// copies the browser made are out of the page's reach.
function leaveEmergencyKit(): void {
  signUpResult.value?.emergencyKitPdf?.fill(0);
  signUpResult.value = null;
  step.value = "checks";
}
</script>

<template>
  <SignInView v-if="(step === 'checks' || step === 'invitePolicy') && !auth.session" />
  <main v-else class="setup">
    <div class="setup-card">
      <p class="product">Hisar · Set up the server</p>
      <p v-if="stepNumber > 0" class="step-number">Step {{ stepNumber }} of {{ setupSteps.length }}</p>
      <p v-if="loadError" class="error" role="alert">{{ loadError }}</p>
      <p v-else-if="!step" class="hint">Loading…</p>
      <p v-else-if="step === 'account' && !inviteToken" class="error" role="alert">{{ invalidSetupLinkMessage }}</p>
      <AccountStep v-else-if="step === 'account'" :invite-token="inviteToken" @signed-up="showEmergencyKit" />
      <EmergencyKitStep v-else-if="step === 'emergencyKit' && signUpResult" :result="signUpResult" @continue="leaveEmergencyKit" />
      <ChecksStep v-else-if="step === 'checks'" @continue="step = 'invitePolicy'" />
      <InvitePolicyStep v-else-if="step === 'invitePolicy'" @done="emit('leave')" />
    </div>
  </main>
</template>

<style>
.setup {
  display: grid;
  place-items: center;
  min-height: 100%;
  padding: 24px;
  background: radial-gradient(700px 500px at 0% 0%, var(--color-page-glow) 0%, transparent 70%), var(--color-sidebar);
}

.setup-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: min(480px, 100%);
  padding: 32px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-panel);
  box-shadow: 0 8px 24px var(--color-shadow);
}

.setup .product,
.setup .step-number {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.setup .step {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.setup h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 650;
  letter-spacing: -0.01em;
}

.setup .subtitle {
  margin: -6px 0 4px;
  color: var(--color-text-muted);
}

.setup .submit {
  margin-top: 4px;
  padding: 10px 12px;
}

.setup .hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 12px;
}

.setup .secret-key {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.setup .secret-key code {
  flex: 1;
  font-size: 15px;
  overflow-wrap: anywhere;
}

.setup .confirmation,
.setup .choice {
  display: flex;
  align-items: center;
  gap: 8px;
}

.setup .confirmation input,
.setup .choice input {
  width: auto;
}

.setup .choices {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  border: none;
}

.setup legend {
  margin-bottom: 6px;
  padding: 0;
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 500;
}

.setup .checks {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.setup .check {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}

.setup .check.failed {
  border-color: var(--color-danger-border);
  background: var(--color-danger-background);
}

.setup .check .mark {
  width: 16px;
  font-weight: 700;
  text-align: center;
}

.setup .check.ok .mark {
  color: var(--color-text-secondary);
}

.setup .check.failed .mark {
  color: var(--color-danger-text);
}

.setup .check-text {
  flex: 1;
}

.setup .check-message {
  display: block;
  color: var(--color-text-muted);
  font-size: 12px;
}

.setup .actions {
  display: flex;
  justify-content: space-between;
  gap: 10px;
}
</style>
