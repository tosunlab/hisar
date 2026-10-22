<script setup lang="ts">
import { onUnmounted, ref } from "vue";
import { getErrorMessage, resumeSetup } from "../../api";
import { bridge, type RecoveryMetadata, type SignUpResult } from "../../bridge/bridge";
import { downloadFile, emergencyKitFileName } from "./setup";

const props = defineProps<{ operations: RecoveryMetadata[]; initialResult?: SignUpResult | null }>();
const emit = defineEmits<{ acknowledged: []; refresh: [] }>();
const password = ref("");
const secretKey = ref("");
const priorSecretKey = ref("");
const isBusy = ref(false);
const errorMessage = ref("");
const result = ref<SignUpResult | null>(props.initialResult ?? null);
const downloadedOperationId = ref("");
const confirmedSaved = ref<Record<string, boolean>>({});

onUnmounted(() => {
  password.value = "";
  secretKey.value = "";
  priorSecretKey.value = "";
  result.value?.emergencyKitPdf?.fill(0);
  result.value = null;
});

async function run(operation: RecoveryMetadata, action: "resume" | "download" | "acknowledge" | "discard"): Promise<void> {
  if (!password.value || isBusy.value) return;
  isBusy.value = true;
  errorMessage.value = "";
  const enteredPassword = password.value;
  const enteredKey = secretKey.value;
  const enteredPriorKey = priorSecretKey.value;
  password.value = "";
  secretKey.value = "";
  priorSecretKey.value = "";
  try {
    if (action === "resume") {
      result.value = await resumeSetup(operation.operationId, enteredPassword, operation.name, enteredKey, enteredPriorKey);
      emit("refresh");
    } else if (action === "download") {
      const pdf = await bridge.exportPendingRecoveryEmergencyKit(operation.operationId, enteredPassword);
      try {
        downloadFile(pdf, emergencyKitFileName, "application/pdf");
        downloadedOperationId.value = operation.operationId;
        confirmedSaved.value[operation.operationId] = false;
        emit("refresh");
      } finally {
        pdf.fill(0);
      }
    } else if (action === "discard") {
      await bridge.discardPreparedRecovery(operation.operationId, enteredPassword);
      emit("acknowledged");
    } else if (confirmedSaved.value[operation.operationId] && (downloadedOperationId.value === operation.operationId || operation.kitSaved)) {
      await bridge.acknowledgePendingRecovery(operation.operationId, enteredPassword);
      result.value = null;
      confirmedSaved.value[operation.operationId] = false;
      emit("acknowledged");
    }
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isBusy.value = false;
  }
}
</script>

<template>
  <section class="step" data-testid="setup-recovery">
    <h1>Finish account setup safely</h1>
    <p class="subtitle">This browser saves a protected operation record, not your Secret Key. Keep the downloaded Emergency Kit outside this browser.</p>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <p v-for="problem in result?.problems" :key="problem" class="error" role="alert">{{ problem }}</p>
    <label><span>Master password used for this account</span><input v-model="password" type="password" name="recoveryPassword" autocomplete="current-password" :disabled="isBusy" /></label>
    <label><span>Secret Key from the saved Emergency Kit, if this page reloaded</span><input v-model="secretKey" type="password" name="recoverySecretKey" autocomplete="off" spellcheck="false" :disabled="isBusy" /></label>
    <label><span>Previous Secret Key, only if requested for an interrupted replacement</span><input v-model="priorSecretKey" type="password" name="recoveryPriorSecretKey" autocomplete="off" spellcheck="false" :disabled="isBusy" /></label>
    <article v-for="operation in props.operations" :key="operation.operationId">
      <h2>{{ operation.email }}</h2>
      <p>{{ operation.origin }}</p>
      <p v-if="result?.pendingOperationId === operation.operationId && result.secretKey" class="secret-key"><code data-testid="recovery-secret-key">{{ result.secretKey }}</code></p>
      <p v-if="operation.phase === 'awaiting-kit'" role="status">The account has not been created. Download the new Emergency Kit and confirm you saved it before account creation can continue.</p>
      <p v-else-if="operation.phase === 'prepared'" role="status">Your kit is confirmed saved. The account has not been created. Choose Verify and resume setup to create it.</p>
      <p v-else-if="operation.phase === 'delivery-pending'" role="status">Account setup is complete. Your Emergency Kit still needs to be confirmed saved.</p>
      <p v-else-if="operation.phase === 'needs-finishing'" role="status">The account exists, but setup still needs to finish. Resume with the same master password.</p>
      <p v-else role="status">The outcome is not yet confirmed. Resume to verify the account before continuing. Do not create another account.</p>
      <p v-for="problem in operation.problems" :key="problem" class="error">{{ problem }}</p>
      <button type="button" :disabled="isBusy || !password || operation.phase === 'awaiting-kit'" @click="run(operation, 'resume')">Verify and resume setup</button>
      <button type="button" :disabled="isBusy || !password || operation.phase === 'outcome-unknown'" @click="run(operation, 'download')">Download recovery Emergency Kit</button>
      <template v-if="downloadedOperationId === operation.operationId || operation.kitSaved">
        <label class="confirmation"><input v-model="confirmedSaved[operation.operationId]" type="checkbox" name="recoveryKitSaved" /><span>I saved this Emergency Kit somewhere safe</span></label>
        <button type="button" :disabled="isBusy || !password || !confirmedSaved[operation.operationId] || (operation.phase !== 'delivery-pending' && operation.phase !== 'awaiting-kit')" @click="run(operation, 'acknowledge')">{{ operation.phase === 'awaiting-kit' ? 'Confirm kit saved before account creation' : 'Confirm saved and continue' }}</button>
      </template>
      <button v-if="operation.phase === 'awaiting-kit' || operation.phase === 'prepared'" type="button" :disabled="isBusy || !password" @click="run(operation, 'discard')">Discard uncommitted setup</button>
    </article>
    <p class="hint">If this page reloaded, enter the Secret Key from the kit to resume. If you closed the page before saving an uncommitted kit, discard that setup and start again. Discard is unavailable once a remote change may have been sent.</p>
  </section>
</template>
