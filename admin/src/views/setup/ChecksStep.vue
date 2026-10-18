<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { confirmSecretsBackup, getErrorMessage, type CheckStatus, type InstanceCheck } from "../../api";
import { finishChecksStep, listSetupChecks } from "./setup";

const emit = defineEmits<{ continue: [] }>();

const statusMarks: Record<CheckStatus, { mark: string; label: string }> = {
  ok: { mark: "✓", label: "OK" },
  warning: { mark: "!", label: "Warning" },
  failed: { mark: "✕", label: "Failed" },
};

const checks = ref<InstanceCheck[] | null>(null);
const isChecking = ref(false);
const isContinuing = ref(false);
const isConfirmingBackup = ref(false);
const errorMessage = ref("");
const failedChecks = computed(() => checks.value?.filter((check) => check.status === "failed") ?? []);

async function runChecks(): Promise<void> {
  isChecking.value = true;
  try {
    checks.value = await listSetupChecks();
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isChecking.value = false;
  }
}

onMounted(runChecks);

async function checkAgain(): Promise<void> {
  errorMessage.value = "";
  await runChecks();
}

async function confirmBackup(): Promise<void> {
  isConfirmingBackup.value = true;
  errorMessage.value = "";
  try {
    const confirmed = await confirmSecretsBackup();
    checks.value = checks.value?.map((check) => (check.id === confirmed.id ? confirmed : check)) ?? null;
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  } finally {
    isConfirmingBackup.value = false;
  }
}

async function submit(): Promise<void> {
  isContinuing.value = true;
  errorMessage.value = "";
  try {
    await finishChecksStep();
    emit("continue");
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
    await runChecks();
  } finally {
    isContinuing.value = false;
  }
}
</script>

<template>
  <form class="step" @submit.prevent="submit">
    <h1>Check the server</h1>
    <p class="subtitle">Fix every failed check before you continue. A warning can wait, and Overview keeps showing it.</p>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <p v-if="!checks" class="hint">Checking…</p>
    <ul v-else class="checks">
      <li v-for="check in checks" :key="check.id" :class="`check ${check.status}`" :data-testid="`check-${check.id}`" :data-status="check.status">
        <span class="mark" role="img" :aria-label="statusMarks[check.status].label">{{ statusMarks[check.status].mark }}</span>
        <span class="check-text">
          <strong>{{ check.title }}</strong>
          <span v-if="check.message" class="check-message">{{ check.message }}</span>
        </span>
        <button v-if="check.id === 'secrets-backup' && check.status !== 'ok'" type="button" :disabled="isConfirmingBackup" @click="confirmBackup">
          {{ isConfirmingBackup ? "Saving…" : "I have a backup" }}
        </button>
      </li>
    </ul>
    <p v-if="failedChecks.length > 0" class="error">Fix {{ failedChecks.map((check) => check.title).join(", ") }} first, then check again.</p>
    <div class="actions">
      <button type="button" :disabled="isChecking" @click="checkAgain">{{ isChecking ? "Checking…" : "Check again" }}</button>
      <button class="primary" type="submit" :disabled="!checks || failedChecks.length > 0 || isContinuing">Continue</button>
    </div>
  </form>
</template>
