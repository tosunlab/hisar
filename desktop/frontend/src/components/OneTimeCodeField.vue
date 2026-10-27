<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { api, type OneTimeCode } from "../api";
import { bindCopyTarget, bindFieldRow } from "../copyrow";
import { copyField } from "../store";
import AppIcon from "./AppIcon.vue";
import SecretReferenceButton from "./SecretReferenceButton.vue";

const properties = defineProps<{ vaultID: string; itemID: string }>();

const oneTimeCode = ref<OneTimeCode | null>(null);
const now = ref(Date.now());
const isUnavailable = ref(false);
let clockTimer: ReturnType<typeof setInterval> | undefined;
let isLoading = false;

const secondsLeft = computed(() => {
  const expiresAt = Date.parse(oneTimeCode.value?.expiresAt ?? "");
  return Number.isNaN(expiresAt) ? 0 : Math.max(0, Math.ceil((expiresAt - now.value) / 1000));
});
const formattedCode = computed(() => {
  const code = oneTimeCode.value?.code ?? "";
  const middle = Math.ceil(code.length / 2);
  return `${code.slice(0, middle)} ${code.slice(middle)}`.trim();
});
const ringOffset = computed(() => {
  const period = oneTimeCode.value?.period || 30;
  return (1 - secondsLeft.value / period) * ringLength;
});
const ringLength = 2 * Math.PI * 8;

async function loadCode(): Promise<void> {
  if (isLoading) {
    return;
  }
  isLoading = true;
  try {
    oneTimeCode.value = await api.getOneTimeCode(properties.vaultID, properties.itemID);
    isUnavailable.value = false;
  } catch {
    isUnavailable.value = true;
  } finally {
    isLoading = false;
  }
}

function tick(): void {
  now.value = Date.now();
  if (oneTimeCode.value && secondsLeft.value === 0) {
    loadCode();
  }
}

watch(() => [properties.vaultID, properties.itemID], loadCode);

onMounted(() => {
  loadCode();
  clockTimer = setInterval(tick, 1000);
});
onBeforeUnmount(() => clearInterval(clockTimer));
</script>

<template>
  <div class="field" data-testid="one-time-code" v-bind="oneTimeCode ? bindFieldRow('one-time code', 'One-time code') : {}">
    <div class="field-copy-target" v-bind="oneTimeCode ? bindCopyTarget() : {}">
    <span v-if="oneTimeCode" class="copy-prefix">Copy </span><span class="field-label">One-time code</span>
    <span class="field-value code-value">
      <template v-if="oneTimeCode && !isUnavailable">
        <span class="monospace code" data-testid="one-time-code-value">{{ formattedCode }}</span>
        <svg class="ring" :class="{ ending: secondsLeft <= 5 }" width="20" height="20" viewBox="0 0 20 20" :aria-label="`${secondsLeft} seconds left`" role="img">
          <circle cx="10" cy="10" r="8" class="ring-track" />
          <circle cx="10" cy="10" r="8" class="ring-progress" :stroke-dasharray="ringLength" :stroke-dashoffset="ringOffset" />
        </svg>
        <span class="muted seconds" data-testid="one-time-code-seconds">{{ secondsLeft }}s</span>
      </template>
      <span v-else-if="isUnavailable" class="muted">The one-time code could not be read.</span>
    </span>
    </div>
    <span class="field-actions">
      <button class="ghost icon-button" aria-label="Copy one-time code" title="Copy" :disabled="!oneTimeCode" @click="copyField('one-time code', 'One-time code')">
        <AppIcon name="copy" />
      </button>
      <SecretReferenceButton field-name="one-time code" label="one-time code" />
    </span>
  </div>
</template>

<style scoped>
.field-copy-target {
  display: grid;
  grid-column: 1 / span 2;
  grid-template-columns: subgrid;
  align-items: center;
  min-width: 0;
}

[data-copy-target]:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: -2px;
}

/* The detail panel's scoped row styles reach only this component's root, so the label and actions match them here. */
.field-label {
  font-size: var(--font-size-12);
  color: var(--color-text-muted);
}

.field-actions {
  display: flex;
  gap: 2px;
  opacity: 0.55;
  transition: opacity 120ms;
}

.field:hover .field-actions,
.field:focus-within .field-actions {
  opacity: 1;
}

.code-value {
  display: flex;
  align-items: center;
  gap: 10px;
}

.code {
  font-size: var(--font-size-17);
  letter-spacing: 0.08em;
}

.ring {
  transform: rotate(-90deg);
}

.ring-track {
  fill: none;
  stroke: var(--color-border-strong);
  stroke-width: 2.5;
}

.ring-progress {
  fill: none;
  stroke: var(--color-text-secondary);
  stroke-width: 2.5;
  stroke-linecap: round;
  transition: stroke-dashoffset 900ms linear;
}

.ring.ending .ring-progress {
  stroke: var(--color-warning);
}

@media (prefers-reduced-motion: reduce) {
  .ring-progress {
    transition: none;
  }
}

.seconds {
  font-size: var(--font-size-12);
  font-variant-numeric: tabular-nums;
}
</style>
