<script setup lang="ts">
import { computed, useId } from "vue";
import { hideEmergencyKitReminder, openEmergencyKit, store } from "../store";
import { getThisDeviceName } from "../util";
import AppIcon from "./AppIcon.vue";

const titleID = useId();
const kit = computed(() => store.status.emergencyKit);
const device = getThisDeviceName();
const copy = computed(() => {
  if (kit.value?.savedBy) {
    const delivery = { file: "Emergency Kit saved.", print: "Emergency Kit sent to the printer.", share: "Emergency Kit shared." }[kit.value.savedBy] ?? "";
    return { title: "Confirm your Emergency Kit", body: `${delivery} A copy saved only on ${device} is lost with it. Print it, or keep another copy somewhere else.` };
  }
  if (kit.value?.kind === "replace-secret-key") return { title: "Save your new Emergency Kit", body: `Your old Emergency Kit no longer works. Save the new one where you can reach it without ${device}.` };
  if (kit.value?.reminder === "14-days") return { title: "Save your Emergency Kit now", body: `It's been at least 2 weeks. Save a copy you can reach without ${device} so losing your devices doesn't mean losing access.` };
  if (kit.value?.reminder === "3-days") return { title: "Your Emergency Kit still isn't saved", body: "You haven't saved an Emergency Kit yet. Losing access to your signed-in devices could lock you out." };
  return { title: "Save your Emergency Kit", body: `Save a copy you can reach without ${device}. Losing access to your signed-in devices before you save it could lock you out.` };
});
</script>

<template>
  <section v-if="kit && !kit.isReminderHidden" class="emergency-kit-banner" :class="{ urgent: kit.reminder === '14-days' && !kit.savedBy }" role="region" :aria-labelledby="titleID" data-testid="emergency-kit-banner">
    <div class="heading"><AppIcon name="warning" :size="16" /><h2 :id="titleID">{{ copy.title }}</h2></div>
    <p>{{ copy.body }}</p>
    <div class="actions">
      <button @click="openEmergencyKit">{{ kit.savedBy ? 'Continue to confirmation' : 'Save Emergency Kit…' }}</button>
      <button class="ghost" aria-label="Hide until Hisar locks" @click="hideEmergencyKitReminder">Not now</button>
    </div>
  </section>
</template>

<style scoped>
.emergency-kit-banner { flex: none; min-width: 0; margin: 12px 12px 0; padding: 12px; border: 1px solid var(--color-warning-border); border-radius: 10px; background: var(--color-warning-background); color: var(--color-warning-text); overflow-wrap: anywhere; }
.urgent { border-width: 2px; }
.heading { display: flex; align-items: flex-start; gap: 8px; }
.heading :deep(.icon) { flex: none; margin-top: 2px; }
h2 { margin: 0; font-size: var(--font-size-13); font-weight: 600; }
p { margin: 8px 0; font-size: var(--font-size-12); }
.actions { display: flex; flex-wrap: wrap; gap: 8px; }
.actions button { min-width: 0; height: auto; white-space: normal; }
@container (max-width: 279px) { .actions { flex-direction: column; align-items: stretch; } }
</style>
