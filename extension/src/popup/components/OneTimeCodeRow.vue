<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { computeSecondsLeft, formatOneTimeCode } from "../../shared/one-time-code";
import type { LoginSummary, OneTimeCode } from "../../shared/protocol";
import { describeError, sendToBackground } from "../api";
import { writeToClipboard } from "../clipboard";
import AppIcon from "./AppIcon.vue";

// requestCode, when given, reads the code in place of the tab's login (the item view, for an item of any site).
const props = withDefaults(
  defineProps<{
    tabId?: number;
    login: Pick<LoginSummary, "vaultID" | "itemID">;
    requestCode?: () => Promise<OneTimeCode>;
    now?: () => number;
    tickMilliseconds?: number;
  }>(),
  {
    now: () => Date.now(),
    tickMilliseconds: 1000,
  },
);
const emit = defineEmits<{ notice: [message: string] }>();

const ringLength = 2 * Math.PI * 7;
const oneTimeCode = ref<OneTimeCode>();
const errorMessage = ref("");
const currentTime = ref(props.now());
let clockTimer: ReturnType<typeof setInterval> | undefined;
let isLoading = false;

const secondsLeft = computed(() => (oneTimeCode.value ? computeSecondsLeft(oneTimeCode.value.expiresAt, currentTime.value) : 0));
const ringOffset = computed(() => (1 - secondsLeft.value / (oneTimeCode.value?.period || 30)) * ringLength);

async function loadCode(): Promise<void> {
  if (isLoading) return;
  isLoading = true;
  try {
    oneTimeCode.value = props.requestCode
      ? await props.requestCode()
      : await sendToBackground<OneTimeCode>({
          type: "popup.getOneTimeCode",
          tabId: props.tabId ?? -1,
          vaultID: props.login.vaultID,
          itemID: props.login.itemID,
        });
    errorMessage.value = "";
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isLoading = false;
  }
}

function tick(): void {
  currentTime.value = props.now();
  if (oneTimeCode.value && secondsLeft.value === 0) void loadCode();
}

async function copyCode(): Promise<void> {
  if (!oneTimeCode.value) return;
  try {
    await writeToClipboard(oneTimeCode.value.code);
    emit("notice", "One-time code copied");
  } catch (error) {
    errorMessage.value = describeError(error);
  }
}

onMounted(() => {
  void loadCode();
  clockTimer = setInterval(tick, props.tickMilliseconds);
});
onBeforeUnmount(() => clearInterval(clockTimer));
</script>

<template>
  <div class="code-row" data-testid="one-time-code">
    <template v-if="oneTimeCode">
      <svg class="ring" :class="{ ending: secondsLeft <= 5 }" width="16" height="16" viewBox="0 0 16 16" role="img" :aria-label="`${secondsLeft} seconds left`">
        <circle cx="8" cy="8" r="7" class="ring-track" />
        <circle cx="8" cy="8" r="7" class="ring-progress" :stroke-dasharray="ringLength" :stroke-dashoffset="ringOffset" />
      </svg>
      <span class="code" data-testid="one-time-code-value">{{ formatOneTimeCode(oneTimeCode.code) }}</span>
      <span class="seconds muted" data-testid="one-time-code-seconds">{{ secondsLeft }}s</span>
      <span class="spacer"></span>
      <button
        type="button"
        class="ghost icon-button"
        title="Copy one-time code"
        aria-label="Copy one-time code"
        data-testid="copy-one-time-code"
        @click="copyCode"
      >
        <AppIcon name="copy" />
      </button>
    </template>
    <span v-else-if="errorMessage" class="error">{{ errorMessage }}</span>
    <span v-else class="muted">Loading the one-time code…</span>
  </div>
</template>

<style scoped>
.code-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 28px;
  margin: 0 0 0 42px;
  font-size: 12px;
}

.code {
  font-family: ui-monospace, "SF Mono", SFMono-Regular, Menlo, monospace;
  font-size: 15px;
  letter-spacing: 0.08em;
  color: var(--color-text);
}

.seconds {
  font-variant-numeric: tabular-nums;
}

.spacer {
  flex: 1;
}

.ring {
  flex: none;
  transform: rotate(-90deg);
}

.ring-track {
  fill: none;
  stroke: var(--color-border-strong);
  stroke-width: 2;
}

.ring-progress {
  fill: none;
  stroke: var(--color-text-secondary);
  stroke-width: 2;
  stroke-linecap: round;
  transition: stroke-dashoffset 900ms linear;
}

.ring.ending .ring-progress {
  stroke: var(--color-warning);
}
</style>
