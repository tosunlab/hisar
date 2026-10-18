<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import type { SignUpResult } from "../../bridge/bridge";
import { downloadFile, emergencyKitFileName } from "./setup";

const props = defineProps<{ result: SignUpResult }>();
const emit = defineEmits<{ continue: [] }>();

const isDownloaded = ref(false);
const isSaved = ref(false);
const copyMessage = ref("");
const isConfirmed = computed(() => isSaved.value && (!props.result.emergencyKitPdf || isDownloaded.value));

// The Secret Key exists only in this page until it is saved, so leaving or reloading asks first.
function warnBeforeUnload(event: BeforeUnloadEvent): void {
  if (!isConfirmed.value) {
    event.preventDefault();
  }
}

onMounted(() => window.addEventListener("beforeunload", warnBeforeUnload));
onUnmounted(() => window.removeEventListener("beforeunload", warnBeforeUnload));

function downloadEmergencyKit(): void {
  if (!props.result.emergencyKitPdf) {
    return;
  }
  downloadFile(props.result.emergencyKitPdf, emergencyKitFileName, "application/pdf");
  isDownloaded.value = true;
}

async function copySecretKey(): Promise<void> {
  try {
    await navigator.clipboard.writeText(props.result.secretKey);
    copyMessage.value = "Copied";
  } catch {
    copyMessage.value = "Select the Secret Key and copy it.";
  }
}
</script>

<template>
  <section class="step">
    <h1>Save your Secret Key</h1>
    <p class="subtitle">
      You need it, together with your master password, to sign in on a new device. The Emergency Kit holds your Secret Key and has a space to write your
      master password.
    </p>
    <p v-if="result.personalVaultError" class="error" role="alert" data-testid="personal-vault-error">
      Your account was created, but the Personal vault could not be created ({{ result.personalVaultError }}). You can create a vault later from the
      Hisar apps. Save your Secret Key now.
    </p>
    <div class="secret-key">
      <code class="monospace" data-testid="secret-key">{{ result.secretKey }}</code>
      <button type="button" class="ghost" @click="copySecretKey">Copy</button>
    </div>
    <p v-if="copyMessage" class="hint" role="status">{{ copyMessage }}</p>
    <template v-if="result.emergencyKitPdf">
      <button type="button" class="download" @click="downloadEmergencyKit">Download Emergency Kit</button>
      <label class="confirmation">
        <input v-model="isSaved" type="checkbox" name="isSaved" />
        <span>I saved my Emergency Kit</span>
      </label>
    </template>
    <template v-else>
      <p class="error" role="alert">The Emergency Kit could not be created ({{ result.emergencyKitError }}). Write down the Secret Key above.</p>
      <label class="confirmation">
        <input v-model="isSaved" type="checkbox" name="isSaved" />
        <span>I wrote down my Secret Key</span>
      </label>
    </template>
    <button class="primary submit" type="button" :disabled="!isConfirmed" @click="emit('continue')">Continue</button>
  </section>
</template>
