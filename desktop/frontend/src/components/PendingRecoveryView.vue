<script setup lang="ts">
import { onUnmounted, ref, watch } from "vue";
import { acknowledgePendingRecovery, discardPreparedRecovery, resumePendingRecovery, savePendingRecoveryKit, store } from "../store";
import type { PendingRecoverySummary } from "../api";
import HisarLogo from "./HisarLogo.vue";

const password = ref("");
const secretKey = ref("");
const priorSecretKey = ref("");
const savedOperationID = ref("");
const isSavedConfirmed = ref<Record<string, boolean>>({});
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
      <label><span>Master password for this operation</span><input v-model="password" name="pendingRecoveryPassword" type="password" autocomplete="current-password" :disabled="store.isBusy" /></label>
      <label><span>Secret Key from the saved Emergency Kit, if this app restarted</span><input v-model="secretKey" name="pendingRecoverySecretKey" type="password" autocomplete="off" spellcheck="false" :disabled="store.isBusy" /></label>
      <label><span>Previous Secret Key, only if requested for an interrupted replacement</span><input v-model="priorSecretKey" name="pendingRecoveryPriorSecretKey" type="password" autocomplete="off" spellcheck="false" :disabled="store.isBusy" /></label>
      <article v-for="operation in store.pendingRecovery" :key="operation.operationId" data-testid="pending-operation">
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
        <div class="row">
          <button :disabled="store.isBusy || !password || operation.phase === 'awaiting-kit'" @click="run(operation, 'resume')">Verify and resume</button>
          <button :disabled="store.isBusy || !password || operation.phase === 'outcome-unknown'" @click="run(operation, 'save')">Save recovery Emergency Kit…</button>
        </div>
        <template v-if="savedOperationID === operation.operationId || operation.kitSaved">
          <label class="check-row"><input v-model="isSavedConfirmed[operation.operationId]" name="pendingRecoveryKitSaved" type="checkbox" /><span>I saved the Emergency Kit somewhere safe</span></label>
          <button class="primary" :disabled="store.isBusy || !password || !isSavedConfirmed[operation.operationId] || (operation.phase !== 'delivery-pending' && operation.phase !== 'awaiting-kit')" @click="run(operation, 'acknowledge')">Confirm saved</button>
        </template>
        <button v-if="operation.phase === 'awaiting-kit' || operation.phase === 'prepared'" class="ghost" :disabled="store.isBusy || !password" @click="run(operation, 'discard')">Discard uncommitted setup</button>
      </article>
      <button class="ghost" :disabled="store.isBusy" @click="store.isRecoveryOpen = false">Finish later</button>
      <p class="hint muted">Finishing later keeps the recovery record. You can reopen recovery from this window.</p>
    </div>
  </main>
</template>

<style scoped>
.recovery { display: grid; place-items: center; height: 100%; overflow: auto; padding: 24px; background: var(--color-page); }
.recovery-card { display: flex; flex-direction: column; gap: 14px; width: min(620px, 100%); }
h1 { font-size: 22px; }
h2 { font-size: 15px; }
p { margin: 0; }
article { display: flex; flex-direction: column; gap: 12px; padding: 16px; border: 1px solid var(--color-border); border-radius: 8px; }
</style>
