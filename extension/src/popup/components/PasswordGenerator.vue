<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { closePopup, describeError, sendToBackground } from "../api";
import { writeToClipboard } from "../clipboard";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{ tabId: number | undefined }>();
const emit = defineEmits<{ notice: [message: string] }>();

const length = ref(24);
const includeDigits = ref(true);
const includeSymbols = ref(true);
const generatedPassword = ref("");
const errorMessage = ref("");

async function generatePassword(): Promise<void> {
  try {
    generatedPassword.value = await sendToBackground<string>({
      type: "popup.generatePassword",
      options: { length: length.value, digits: includeDigits.value, symbols: includeSymbols.value },
    });
    errorMessage.value = "";
  } catch (error) {
    errorMessage.value = describeError(error);
  }
}

async function copyPassword(): Promise<void> {
  try {
    await writeToClipboard(generatedPassword.value);
    emit("notice", "Password copied");
  } catch (error) {
    errorMessage.value = describeError(error);
  }
}

async function fillPassword(): Promise<void> {
  if (props.tabId === undefined) return;
  try {
    await sendToBackground({ type: "popup.fillPassword", tabId: props.tabId, password: generatedPassword.value });
    closePopup();
  } catch (error) {
    errorMessage.value = describeError(error);
  }
}

onMounted(generatePassword);
watch([length, includeDigits, includeSymbols], generatePassword);
</script>

<template>
  <div class="generator" data-testid="generator">
    <div class="generated-row">
      <p class="generated monospace" data-testid="generated-password">{{ generatedPassword }}</p>
      <button type="button" class="ghost icon-button" aria-label="Regenerate" title="Regenerate" @click="generatePassword">
        <AppIcon name="refresh" />
      </button>
    </div>
    <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    <label class="length">
      <span>Length</span>
      <input v-model.number="length" name="length" type="range" min="12" max="64" data-testid="length" />
      <span class="length-value">{{ length }}</span>
    </label>
    <div class="toggles">
      <label class="toggle"><input v-model="includeDigits" name="digits" type="checkbox" /> <span>Digits</span></label>
      <label class="toggle"><input v-model="includeSymbols" name="symbols" type="checkbox" /> <span>Symbols</span></label>
    </div>
    <div class="row actions">
      <button type="button" class="full-width" :disabled="!generatedPassword" data-testid="copy-generated" @click="copyPassword">
        <AppIcon name="copy" /> Copy
      </button>
      <button
        type="button"
        class="primary full-width"
        :disabled="!generatedPassword || tabId === undefined"
        title="Fill into the focused field on the page"
        data-testid="fill-generated"
        @click="fillPassword"
      >
        <AppIcon name="fill" /> Fill field
      </button>
    </div>
  </div>
</template>

<style scoped>
.generated-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.generated {
  flex: 1;
  min-height: 22px;
  margin: 0;
  font-size: 14px;
  overflow-wrap: anywhere;
  user-select: text;
}

.generator > .error {
  margin-top: 10px;
}

.length {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 14px 0 10px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.length input {
  flex: 1;
}

.length-value {
  min-width: 22px;
  font-variant-numeric: tabular-nums;
  color: var(--color-text);
}

.toggles {
  display: flex;
  gap: 18px;
}

.toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.actions {
  margin-top: 14px;
}
</style>
