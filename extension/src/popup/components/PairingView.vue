<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { describeError, sendToBackground } from "../api";
import AppIcon from "./AppIcon.vue";

const emit = defineEmits<{ paired: [] }>();

const step = ref<"start" | "code">("start");
const code = ref("");
const isBusy = ref(false);
const errorMessage = ref("");
const codeInput = ref<HTMLInputElement>();

const normalizedCode = computed(() => code.value.replace(/\D/g, ""));

async function startPairing(): Promise<void> {
  isBusy.value = true;
  errorMessage.value = "";
  try {
    await sendToBackground({ type: "popup.startPairing" });
    step.value = "code";
    code.value = "";
    await nextTick();
    codeInput.value?.focus();
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isBusy.value = false;
  }
}

async function finishPairing(): Promise<void> {
  if (normalizedCode.value.length !== 6) return;
  isBusy.value = true;
  errorMessage.value = "";
  try {
    await sendToBackground({ type: "popup.finishPairing", code: normalizedCode.value });
    emit("paired");
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isBusy.value = false;
  }
}
</script>

<template>
  <section class="panel" data-testid="pairing-view">
    <template v-if="step === 'start'">
      <h1>Connect to Hisar</h1>
      <p class="subtitle">Pair this browser with the Hisar app on this computer to fill and save logins.</p>
      <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
      <button type="button" class="primary full-width" :disabled="isBusy" data-testid="start-pairing" @click="startPairing">
        <AppIcon name="laptop" /> Pair with Hisar
      </button>
    </template>
    <form v-else @submit.prevent="finishPairing">
      <h1>Enter the pairing code</h1>
      <p class="subtitle">Hisar shows a 6-digit code. Check that it asks to pair this browser, then enter the code here.</p>
      <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
      <input
        ref="codeInput"
        v-model="code"
        class="code monospace"
        type="text"
        inputmode="numeric"
        autocomplete="one-time-code"
        maxlength="7"
        placeholder="000000"
        aria-label="Pairing code"
        data-testid="pairing-code"
      />
      <div class="row actions">
        <button type="button" class="ghost" :disabled="isBusy" @click="startPairing">New code</button>
        <span class="spacer"></span>
        <button type="submit" class="primary" :disabled="isBusy || normalizedCode.length !== 6" data-testid="finish-pairing">
          <AppIcon name="check" /> Pair
        </button>
      </div>
    </form>
  </section>
</template>

<style scoped>
.code {
  margin-bottom: 14px;
  font-size: 22px;
  letter-spacing: 0.3em;
  text-align: center;
}
</style>
