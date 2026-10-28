<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { store } from "../store";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{ secretName?: string }>();
const root = ref<HTMLElement | null>(null);
const isRevealed = ref(false);
let visibilityObserver: MutationObserver | undefined;

function conceal(): void {
  isRevealed.value = false;
  const input = root.value?.querySelector("input");
  if (input) input.type = "password";
}

async function toggle(): Promise<void> {
  const input = root.value?.querySelector("input");
  if (!input) return;
  const { selectionStart, selectionEnd, selectionDirection } = input;
  isRevealed.value = !isRevealed.value;
  input.type = isRevealed.value ? "text" : "password";
  await nextTick();
  if (selectionStart !== null && selectionEnd !== null) input.setSelectionRange(selectionStart, selectionEnd, selectionDirection ?? undefined);
}

watch(() => [store.status.state, store.isBusy, store.isSettingsOpen], conceal, { flush: "sync" });
onMounted(() => {
  document.addEventListener("submit", conceal, true);
  visibilityObserver = new MutationObserver(() => {
    if (root.value?.closest('[hidden], [style*="display: none"]')) conceal();
  });
  visibilityObserver.observe(document.body, { attributes: true, attributeFilter: ["style", "hidden"], subtree: true });
});
onBeforeUnmount(() => {
  conceal();
  document.removeEventListener("submit", conceal, true);
  visibilityObserver?.disconnect();
});
</script>

<template>
  <span ref="root" class="credential-field">
    <slot />
    <button type="button" class="ghost icon-button credential-toggle" :aria-label="`${isRevealed ? 'Hide' : 'Show'} ${props.secretName ?? 'master password'}`" :aria-pressed="isRevealed" @mousedown.prevent @click.prevent="toggle">
      <AppIcon :name="isRevealed ? 'eyeOff' : 'eye'" />
    </button>
  </span>
</template>

<style scoped>
.credential-field {
  position: relative;
  display: inline-flex;
  flex: 1;
  width: 100%;
  min-width: 0;
}
.credential-field :slotted(input) {
  padding-right: 38px;
  min-width: 0;
}
.credential-toggle {
  position: absolute;
  right: 3px;
  top: 50%;
  transform: translateY(-50%);
}
</style>
