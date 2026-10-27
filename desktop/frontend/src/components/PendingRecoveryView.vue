<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { acknowledgePendingRecovery, discardPreparedRecovery, resumePendingRecovery, savePendingRecoveryKit, store } from "../store";
import type { PendingRecoverySummary } from "../api";
import HisarLogo from "./HisarLogo.vue";

const password = ref("");
const secretKey = ref("");
const priorSecretKey = ref("");
const savedOperationID = ref("");
const isSavedConfirmed = ref<Record<string, boolean>>({});
const selectedID = ref("");
const selected = computed(() => store.pendingRecovery.find((operation) => operation.operationId === selectedID.value));
const nextAction = computed(() => {
  const operation = selected.value;
  if (!operation) return null;
  if (operation.phase === "manual-unlock") return { kind: "resume" as const, label: "Sign in with saved Emergency Kit" };
  if (["prepared", "needs-finishing", "outcome-unknown"].includes(operation.phase)) return { kind: "resume" as const, label: "Continue" };
  if (operation.kitSaved || savedOperationID.value === operation.operationId) return { kind: "acknowledge" as const, label: "Confirm saved" };
  return { kind: "save" as const, label: "Save Emergency Kit…" };
});
watch(() => store.pendingRecovery, (records) => {
  if (!records.some((operation) => operation.operationId === selectedID.value)) {
    selectedID.value = store.recoveryFactor?.operationId ?? (records.length === 1 ? records[0].operationId : "");
  }
}, { immediate: true });
watch(selectedID, () => {
  clearFactors();
  savedOperationID.value = "";
  isSavedConfirmed.value = {};
});
function clearFactors(): void {
  password.value = "";
  secretKey.value = "";
  priorSecretKey.value = "";
}
onUnmounted(() => { clearFactors(); store.recoveryFactor = null; });
watch(() => store.recoveryEpoch, () => {
  clearFactors();
  savedOperationID.value = "";
  isSavedConfirmed.value = {};
});

async function run(operation: PendingRecoverySummary, action: "resume" | "save" | "acknowledge" | "discard"): Promise<void> {
  if (!password.value || store.isBusy) return;
  const enteredPassword = password.value;
  const enteredKey = secretKey.value;
  const enteredPriorKey = priorSecretKey.value;
  clearFactors();
  if (action === "resume") {
    await resumePendingRecovery(operation.operationId, enteredPassword, enteredKey, enteredPriorKey);
  } else if (action === "save") {
    if (await savePendingRecoveryKit(operation.operationId, enteredPassword)) {
      savedOperationID.value = operation.operationId;
      isSavedConfirmed.value[operation.operationId] = false;
    }
  } else if (action === "discard") {
    await discardPreparedRecovery(operation.operationId, enteredPassword);
  } else if (isSavedConfirmed.value[operation.operationId] && (savedOperationID.value === operation.operationId || operation.kitSaved)) {
    await acknowledgePendingRecovery(operation.operationId, enteredPassword);
    isSavedConfirmed.value[operation.operationId] = false;
  }
}
</script>

<template>
  <main class="recovery" data-testid="pending-recovery">
    <div class="recovery-card">
      <HisarLogo />
      <h1>Finish saving your account recovery</h1>
      <p>Choose the operation you want to finish. Nothing is resumed automatically.</p>
      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      <label v-if="store.pendingRecovery.length > 1">
        <span>Choose an account</span>
        <select v-model="selectedID" :disabled="store.isBusy" data-testid="pending-recovery-chooser">
          <option value="" disabled>Choose an account</option>
          <option v-for="operation in store.pendingRecovery" :key="operation.operationId" :value="operation.operationId">{{ operation.email }} · {{ operation.origin }}</option>
        </select>
      </label>
      <article v-if="selected" :key="selected.operationId" data-testid="pending-operation">
        <template v-for="operation in [selected]" :key="operation.operationId">
        <h2>{{ operation.kind === 'signup' ? 'Account setup' : 'Secret Key replacement' }} · {{ operation.email }}</h2>
        <p class="muted">{{ operation.origin }}</p>
        <p v-if="store.recoveryFactor?.operationId === operation.operationId" class="monospace" data-testid="recovery-secret-key">{{ store.recoveryFactor.secretKey }}</p>
        <p v-if="operation.phase === 'awaiting-kit'" role="status">The account change has not been sent. Save the new Emergency Kit and confirm you have it before continuing.</p>
        <p v-else-if="operation.phase === 'prepared'" role="status">Your kit is confirmed saved. The account change has not been sent. Choose Verify and resume to commit it.</p>
        <p v-else-if="operation.phase === 'delivery-pending'" role="status">The operation completed. Save and confirm the Emergency Kit before dismissing this recovery record.</p>
        <p v-else-if="operation.phase === 'needs-finishing'" role="status">The account was changed, but setup still needs finishing. Verify and resume with the same master password.</p>
        <p v-else-if="operation.phase === 'manual-unlock'" role="status">The keychain is unavailable. Sign in with the Secret Key from your saved Emergency Kit and your master password.</p>
        <p v-else role="status">The server outcome is not confirmed. Verify and resume before assuming the change succeeded or failed.</p>
        <p class="hint">Verify and resume signs in to this account. If a different account is open, it will be replaced only when you choose this action.</p>
        <p v-if="!operation.keychainProtected" class="hint">This device does not persist the new Secret Key. After a restart, enter the Secret Key from your saved kit to resume. If you closed the app before saving an uncommitted kit, discard that setup and start again.</p>
        <p v-for="problem in operation.problems" :key="problem" class="error">{{ problem }}</p>
        <label><span>Master password for this operation</span><input v-model="password" name="pendingRecoveryPassword" type="password" autocomplete="current-password" :disabled="store.isBusy" /></label>
        <label v-if="!operation.keychainProtected"><span>Secret Key from the saved Emergency Kit, if this app restarted</span><input v-model="secretKey" name="pendingRecoverySecretKey" type="password" autocomplete="off" spellcheck="false" :disabled="store.isBusy" /></label>
        <label v-if="!operation.keychainProtected && operation.kind !== 'signup'"><span>Previous Secret Key, only if requested for an interrupted replacement</span><input v-model="priorSecretKey" name="pendingRecoveryPriorSecretKey" type="password" autocomplete="off" spellcheck="false" :disabled="store.isBusy" /></label>
        <template v-if="nextAction?.kind === 'acknowledge'">
          <label class="check-row"><input v-model="isSavedConfirmed[operation.operationId]" name="pendingRecoveryKitSaved" type="checkbox" /><span>I saved the Emergency Kit somewhere safe</span></label>
        </template>
        <p v-if="!password" class="hint">Enter your master password to continue.</p>
        <button v-if="nextAction" class="primary" :disabled="store.isBusy || !password || (nextAction.kind === 'acknowledge' && !isSavedConfirmed[operation.operationId])" @click="run(operation, nextAction.kind)">{{ nextAction.label }}</button>
        <button v-if="operation.phase === 'awaiting-kit' || operation.phase === 'prepared'" class="ghost" :disabled="store.isBusy || !password" @click="run(operation, 'discard')">Discard uncommitted setup</button>
        </template>
      </article>
      <button class="ghost" :disabled="store.isBusy" @click="store.isRecoveryOpen = false">Finish later</button>
      <p class="hint muted">Finishing later keeps the recovery record. You can reopen recovery from this window.</p>
    </div>
  </main>
</template>

<style scoped>
.recovery { display: grid; place-items: center; height: 100%; overflow: auto; padding: 24px; background: var(--color-page); }
.recovery-card { display: flex; flex-direction: column; gap: 14px; width: min(620px, 100%); padding: 28px; border: 1px solid var(--color-border); border-radius: var(--radius); background: var(--color-surface); box-shadow: 0 8px 28px var(--color-shadow); }
h1 { font-size: var(--font-size-22); }
h2 { font-size: var(--font-size-15); }
p { margin: 0; }
article { display: flex; flex-direction: column; gap: 12px; padding: 16px; border: 1px solid var(--color-border); border-radius: 8px; }
</style>
