<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { api, getErrorMessage } from "../api";

const emit = defineEmits<{ use: [password: string]; close: [] }>();

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
  <div class="generator">
    <p class="generated monospace" data-testid="generated-password">{{ generatedPassword }}</p>
    <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    <label class="row">
      <span class="length-label">Length {{ length }}</span>
      <input v-model.number="length" name="length" type="range" min="12" max="64" />
    </label>
    <div class="row options">
      <label class="row"><input v-model="includeDigits" name="digits" type="checkbox" /> Digits</label>
      <label class="row"><input v-model="includeSymbols" name="symbols" type="checkbox" /> Symbols</label>
    </div>
    <div class="row">
      <button type="button" @click="generatePassword">Regenerate</button>
      <div class="spacer"></div>
      <button type="button" @click="emit('close')">Cancel</button>
      <button type="button" class="primary" :disabled="!generatedPassword" @click="emit('use', generatedPassword)">
        Use password
      </button>
    </div>
  </div>
</template>

<style scoped>
.generator {
  margin: 0 0 12px;
  padding: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
}

.generated {
  margin: 0 0 12px;
  font-size: 15px;
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}

.length-label {
  min-width: 80px;
  margin: 0;
}

.options {
  gap: 16px;
}

.options input {
  width: auto;
}

label.row {
  margin-bottom: 8px;
}
</style>
