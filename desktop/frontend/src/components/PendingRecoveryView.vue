<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { acknowledgePendingRecovery, discardPreparedRecovery, resumePendingRecovery, savePendingRecoveryKit, store } from "../store";
import type { PendingRecoverySummary } from "../api";
import HisarLogo from "./HisarLogo.vue";
import { recoveryHeading, recoveryHost, recoveryStatus } from "../pendingRecoveryCopy";
import SecretKeyBlock from "./SecretKeyBlock.vue";

type RecoveryFactors = { password: string; secretKey: string; priorSecretKey: string; needsPriorSecretKey: boolean };
const factorsByOperation = ref<Record<string, RecoveryFactors>>({});
const savedOperationID = ref("");
const isSavedConfirmed = ref<Record<string, boolean>>({});
function factorsFor(operation: PendingRecoverySummary): RecoveryFactors {
  return factorsByOperation.value[operation.operationId] ??= { password: "", secretKey: "", priorSecretKey: "", needsPriorSecretKey: false };
}
const selectedID = ref(store.pendingRecovery[0]?.operationId ?? "");
const selected = computed(() => store.pendingRecovery.find(operation => operation.operationId === selectedID.value));
watch(() => store.pendingRecovery.map(operation => operation.operationId), ids => {
  if (!ids.includes(selectedID.value)) selectedID.value = ids[0] ?? "";
});
function clearFactors(): void {
  factorsByOperation.value = {};
}
watch(selectedID, () => { clearFactors(); store.recoveryFactor = null; });
watch(() => [store.status.email, store.status.server], clearFactors);
watch(() => store.status.state, state => {
  if (state === "locked" || state === "signed-out") clearFactors();
});
watch(() => store.isRecoveryOpen, open => { if (!open) clearFactors(); });
onUnmounted(() => { clearFactors(); store.recoveryFactor = null; });
watch(() => store.recoveryEpoch, () => {
  clearFactors();
  savedOperationID.value = "";
  isSavedConfirmed.value = {};
});

async function run(operation: PendingRecoverySummary, action: "resume" | "save" | "acknowledge" | "discard"): Promise<void> {
  const factors = factorsFor(operation);
  if (!factors.password || store.isBusy) return;
  const enteredPassword = factors.password;
  const enteredKey = factors.secretKey;
  const enteredPriorKey = factors.priorSecretKey;
  let succeeded = false;
  if (action === "resume") {
    succeeded = await resumePendingRecovery(operation.operationId, enteredPassword, enteredKey, enteredPriorKey);
    if (!succeeded && operation.kind === "replacement" && store.errorMessage === "enter the previous Secret Key to resume this interrupted replacement") {
      factors.needsPriorSecretKey = true;
    }
  } else if (action === "save") {
    if (await savePendingRecoveryKit(operation.operationId, enteredPassword)) {
      savedOperationID.value = operation.operationId;
      isSavedConfirmed.value[operation.operationId] = false;
    }
  } else if (action === "discard") {
    if (await discardPreparedRecovery(operation.operationId, enteredPassword)) clearFactors();
  } else if (isSavedConfirmed.value[operation.operationId] && (savedOperationID.value === operation.operationId || operation.kitSaved)) {
    succeeded = await acknowledgePendingRecovery(operation.operationId, enteredPassword);
    if (succeeded && operation.phase === "delivery-pending") clearFactors();
    if (succeeded) isSavedConfirmed.value[operation.operationId] = false;
  }
  if (store.recoveryPasswordRejected) factors.password = "";
}
</script>

<template>
  <main class="recovery" data-testid="pending-recovery">
    <div class="recovery-card">
      <HisarLogo />
      <h1>Finish account setup</h1>
      <p>Save and confirm your Emergency Kit to finish this setup.</p>
      <div v-if="store.pendingRecovery.length > 1" class="recovery-chooser">
        <button v-for="operation in store.pendingRecovery" :key="operation.operationId" @click="selectedID = operation.operationId">
          <strong>{{ recoveryHeading(operation.kind) }}</strong>
          <span>{{ operation.email }} · {{ recoveryHost(operation.origin) }} ›</span>
        </button>
      </div>
      <p v-if="store.errorMessage" class="error" role="alert">{{ store.errorMessage }}</p>
      <article v-for="operation in selected ? [selected] : []" :key="operation.operationId" data-testid="pending-operation">
        <h2>{{ recoveryHeading(operation.kind) }}</h2>
        <p class="muted">{{ operation.email }} · {{ recoveryHost(operation.origin) }}</p>
        <p><strong>Server</strong> {{ operation.origin }}</p>
        <SecretKeyBlock v-if="store.recoveryFactor?.operationId === operation.operationId" :secret-key="store.recoveryFactor.secretKey" test-id="recovery-secret-key" />
        <p role="status" data-testid="pending-recovery-phase">{{ recoveryStatus(operation.kind, operation.phase) }}</p>
        <p v-if="store.status.state !== 'signed-out' && (store.status.email !== operation.email || store.status.server !== operation.origin)" class="hint">Continuing signs this device in to {{ operation.email }} on {{ recoveryHost(operation.origin) }}.</p>
        <p v-if="!operation.keychainProtected" class="hint">This device does not persist the new Secret Key. After a restart, enter the Secret Key from your saved kit to resume. If you closed the app before saving an uncommitted kit, discard that setup and start again.</p>
        <p v-for="problem in operation.problems" :key="problem" class="error">{{ problem }}</p>
        <label><span>Master password for this operation</span><input v-model="factorsFor(operation).password" name="pendingRecoveryPassword" type="password" autocomplete="current-password" :disabled="store.isBusy" /><span class="hint">Enter your master password to continue.</span></label>
        <label v-if="operation.manualUnlock || !operation.keychainProtected"><span>Secret Key from the saved Emergency Kit</span><input v-model="factorsFor(operation).secretKey" name="pendingRecoverySecretKey" type="text" class="monospace" placeholder="H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX" autocomplete="off" spellcheck="false" :disabled="store.isBusy" /></label>
        <label v-if="operation.kind === 'replacement' && factorsFor(operation).needsPriorSecretKey"><span>Previous Secret Key</span><input v-model="factorsFor(operation).priorSecretKey" name="pendingRecoveryPriorSecretKey" type="text" class="monospace" placeholder="H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX" autocomplete="off" spellcheck="false" :disabled="store.isBusy" /></label>
        <div class="row">
          <button :disabled="store.isBusy || !factorsFor(operation).password || operation.phase === 'awaiting-kit'" @click="run(operation, 'resume')">{{ operation.phase === 'manual-unlock' ? 'Sign in with saved Emergency Kit' : 'Continue' }}</button>
          <button :disabled="store.isBusy || !factorsFor(operation).password || operation.phase === 'outcome-unknown'" @click="run(operation, 'save')">Save Emergency Kit…</button>
        </div>
        <template v-if="savedOperationID === operation.operationId || operation.kitSaved">
          <label class="check-row"><input v-model="isSavedConfirmed[operation.operationId]" name="pendingRecoveryKitSaved" type="checkbox" /><span>I saved the Emergency Kit somewhere safe</span></label>
          <button class="primary" :disabled="store.isBusy || !factorsFor(operation).password || !isSavedConfirmed[operation.operationId] || (operation.phase !== 'delivery-pending' && operation.phase !== 'awaiting-kit')" @click="run(operation, 'acknowledge')">Confirm saved</button>
        </template>
        <template v-if="operation.phase === 'awaiting-kit' || operation.phase === 'prepared'">
          <button class="ghost" :disabled="store.isBusy || !factorsFor(operation).password" @click="run(operation, 'discard')">Cancel this setup</button>
          <p class="hint">This removes only this uncommitted setup. No server change has been sent.</p>
        </template>
      </article>
      <button class="ghost" :disabled="store.isBusy" @click="store.isRecoveryOpen = false">Finish later</button>
      <p class="hint muted">You can finish later from the reminder in this window.</p>
    </div>
  </main>
</template>

<style scoped>
.recovery { display: grid; place-items: center; height: 100%; overflow: auto; padding: 24px; background: var(--color-page); }
.recovery-card { display: flex; flex-direction: column; gap: 14px; width: min(620px, 100%); }
h1 { font-size: var(--font-size-22); }
h2 { font-size: var(--font-size-15); }
p { margin: 0; }
article { display: flex; flex-direction: column; gap: 12px; padding: 16px; border: 1px solid var(--color-border); border-radius: 8px; }
</style>
