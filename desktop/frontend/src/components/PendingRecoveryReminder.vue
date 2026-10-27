<script setup lang="ts">
import { computed } from "vue";
import { store } from "../store";
import AppIcon from "./AppIcon.vue";

const reminder = computed(() => {
  const records = store.pendingRecovery;
  if (records.length !== 1) return { label: `${records.length} account setups need attention`, hint: "Opens the account setup chooser" };
  const record = records[0];
  if (record.phase === "manual-unlock") return { label: "Sign in with your saved Emergency Kit.", hint: "Opens sign-in with your saved Emergency Kit" };
  if (record.phase === "awaiting-kit" || record.phase === "delivery-pending") {
    return {
      label: record.kind === "signup" ? `Save your Emergency Kit to finish setting up ${record.email}.` : "Save your new Emergency Kit to finish replacing your Secret Key.",
      hint: "Opens the Emergency Kit step",
    };
  }
  return { label: `Finish setting up ${record.email}.`, hint: "Opens the unfinished account setup" };
});
</script>

<template>
  <button v-if="store.pendingRecovery.length && !store.isRecoveryOpen && !store.pendingSecretKey" class="recovery-reminder" data-testid="open-pending-recovery" :title="reminder.hint" :aria-description="reminder.hint" @click="store.isRecoveryOpen = true">
    <AppIcon name="warning" :size="16" />
    <span>{{ reminder.label }} <span class="continue">Continue…</span></span>
    <AppIcon name="chevronRight" :size="14" />
  </button>
</template>

<style scoped>
.recovery-reminder { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 48px; height: auto; padding: 12px; text-align: left; white-space: normal; }
.recovery-reminder > span { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.recovery-reminder > svg { flex: none; }
.continue { color: var(--color-accent-text); }
</style>
