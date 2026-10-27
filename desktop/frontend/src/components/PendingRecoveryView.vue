<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { acknowledgePendingRecovery, discardPreparedRecovery, resumePendingRecovery, savePendingRecoveryKit, store } from "../store";
import type { PendingRecoverySummary } from "../api";
import HisarLogo from "./HisarLogo.vue";
import { recoveryHeading, recoveryHost, recoveryStatus } from "../pendingRecoveryCopy";

const password = ref("");
const secretKey = ref("");
const priorSecretKey = ref("");
const savedOperationID = ref("");
const isSavedConfirmed = ref<Record<string, boolean>>({});
const selectedID = ref(store.pendingRecovery[0]?.operationId ?? "");
const selected = computed(() => store.pendingRecovery.find(operation => operation.operationId === selectedID.value));
watch(() => store.pendingRecovery.map(operation => operation.operationId), ids => {
  if (!ids.includes(selectedID.value)) selectedID.value = ids[0] ?? "";
});
function clearFactors(): void {
  password.value = "";
  secretKey.value = "";
  priorSecretKey.value = "";
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
  if (!password.value || store.isBusy) return;
  const enteredPassword = password.value;
  const enteredKey = secretKey.value;
  const enteredPriorKey = priorSecretKey.value;
  let succeeded = false;
  if (action === "resume") {
    succeeded = await resumePendingRecovery(operation.operationId, enteredPassword, enteredKey, enteredPriorKey);
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
  if (store.recoveryPasswordRejected) password.value = "";
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
      <label><span>Master password</span><input v-model="password" name="pendingRecoveryPassword" type="password" autocomplete="current-password" :disabled="store.isBusy" /><span class="hint">Enter your master password to continue.</span></label>
      <label><span>Secret Key from the saved Emergency Kit, if this app restarted</span><input v-model="secretKey" name="pendingRecoverySecretKey" type="password" autocomplete="off" spellcheck="false" :disabled="store.isBusy" /></label>
      <label><span>Previous Secret Key, only if requested for an interrupted replacement</span><input v-model="priorSecretKey" name="pendingRecoveryPriorSecretKey" type="password" autocomplete="off" spellcheck="false" :disabled="store.isBusy" /></label>
      <article v-for="operation in selected ? [selected] : []" :key="operation.operationId" data-testid="pending-operation">
        <h2>{{ recoveryHeading(operation.kind) }}</h2>
        <p class="muted">{{ operation.email }} · {{ recoveryHost(operation.origin) }}</p>
        <p><strong>Server</strong> {{ operation.origin }}</p>
        <p v-if="store.recoveryFactor?.operationId === operation.operationId" class="monospace" data-testid="recovery-secret-key">{{ store.recoveryFactor.secretKey }}</p>
        <p role="status" data-testid="pending-recovery-phase">{{ recoveryStatus(operation.kind, operation.phase) }}</p>
        <p v-if="store.status.state !== 'signed-out' && (store.status.email !== operation.email || store.status.server !== operation.origin)" class="hint">Continuing signs this device in to {{ operation.email }} on {{ recoveryHost(operation.origin) }}.</p>
        <p v-if="!operation.keychainProtected" class="hint">This device does not persist the new Secret Key. After a restart, enter the Secret Key from your saved kit to resume. If you closed the app before saving an uncommitted kit, discard that setup and start again.</p>
        <p v-for="problem in operation.problems" :key="problem" class="error">{{ problem }}</p>
        <div class="row">
          <button :disabled="store.isBusy || !password || operation.phase === 'awaiting-kit'" @click="run(operation, 'resume')">{{ operation.phase === 'manual-unlock' ? 'Sign in with saved Emergency Kit' : 'Continue' }}</button>
          <button :disabled="store.isBusy || !password || operation.phase === 'outcome-unknown'" @click="run(operation, 'save')">Save Emergency Kit…</button>
        </div>
        <template v-if="savedOperationID === operation.operationId || operation.kitSaved">
          <label class="check-row"><input v-model="isSavedConfirmed[operation.operationId]" name="pendingRecoveryKitSaved" type="checkbox" /><span>I saved the Emergency Kit somewhere safe</span></label>
          <button class="primary" :disabled="store.isBusy || !password || !isSavedConfirmed[operation.operationId] || (operation.phase !== 'delivery-pending' && operation.phase !== 'awaiting-kit')" @click="run(operation, 'acknowledge')">Confirm saved</button>
        </template>
        <template v-if="operation.phase === 'awaiting-kit' || operation.phase === 'prepared'">
          <button class="ghost" :disabled="store.isBusy || !password" @click="run(operation, 'discard')">Cancel this setup</button>
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
