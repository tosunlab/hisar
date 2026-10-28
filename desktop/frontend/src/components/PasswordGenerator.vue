<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { api, getErrorMessage } from "../api";
import AppIcon from "./AppIcon.vue";
import StrengthMeter from "./StrengthMeter.vue";

withDefaults(defineProps<{ presentation?: "editor" | "standalone" }>(), { presentation: "editor" });
const emit = defineEmits<{ use: [password: string]; copy: [password: string]; close: [] }>();

const length = ref(24);
const includeDigits = ref(true);
const includeSymbols = ref(true);
const generatedPassword = ref("");
const errorMessage = ref("");

async function generatePassword(): Promise<void> {
  try {
    generatedPassword.value = await api.generatePassword({
      length: length.value,
      digits: includeDigits.value,
      symbols: includeSymbols.value,
    });
    errorMessage.value = "";
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  }
}

onMounted(generatePassword);
watch([length, includeDigits, includeSymbols], generatePassword);
</script>

<template>
  <div class="generator" :class="{ standalone: presentation === 'standalone' }">
    <div class="generated-row">
      <p class="generated monospace" data-testid="generated-password">{{ generatedPassword }}</p>
      <button type="button" class="ghost" :class="{ 'icon-button': presentation === 'editor' }" aria-label="Regenerate" title="Regenerate" @click="generatePassword">
        <AppIcon name="refresh" />
        <span v-if="presentation === 'standalone'">Regenerate</span>
      </button>
    </div>
    <StrengthMeter :password="generatedPassword" />
    <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    <div class="options">
      <label class="length">
        <span>Length</span>
        <input v-model.number="length" name="length" type="range" min="8" max="64" />
        <span class="length-value">{{ length }}</span>
      </label>
      <label class="toggle"><input v-model="includeDigits" name="digits" type="checkbox" /> <span>Digits</span></label>
      <label class="toggle"><input v-model="includeSymbols" name="symbols" type="checkbox" /> <span>Symbols</span></label>
    </div>
    <div class="row actions">
      <div class="spacer"></div>
      <button type="button" class="ghost" @click="emit('close')">{{ presentation === "standalone" ? "Close" : "Cancel" }}</button>
      <button v-if="presentation === 'standalone'" type="button" class="primary" :disabled="!generatedPassword" @click="emit('copy', generatedPassword)">
        <AppIcon name="copy" /> Copy
      </button>
      <button v-else type="button" class="primary" :disabled="!generatedPassword" @click="emit('use', generatedPassword)">
        <AppIcon name="check" /> Use password
      </button>
    </div>
  </div>
</template>

<style scoped>
.generator {
  margin: -4px 0 16px;
  padding: 14px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.generator.standalone {
  margin: 0;
}

.generated-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.generated {
  flex: 1;
  margin: 0;
  font-size: var(--font-size-15);
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}

.options {
  display: flex;
  align-items: center;
  gap: 18px;
  margin-top: 14px;
}

.options label {
  margin: 0;
}

.length {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 10px;
}

.length span:first-child {
  margin: 0;
}

.length-value {
  min-width: 22px;
  font-variant-numeric: tabular-nums;
}

.toggle {
  display: flex;
  align-items: center;
  gap: 6px;
}

.toggle span {
  margin: 0;
  font-size: var(--font-size-12);
  font-weight: 400;
  color: var(--color-text-secondary);
}

.actions {
  margin-top: 12px;
}
</style>
