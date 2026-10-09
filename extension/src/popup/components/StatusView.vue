<script setup lang="ts">
import { computed } from "vue";
import type { PopupView } from "../../shared/messages";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{ view: PopupView; detail?: string; isWaiting?: boolean }>();
const emit = defineEmits<{ openHisar: []; unlock: []; refresh: [] }>();

const content = computed(() => {
  switch (props.view) {
    case "desktop-unavailable":
      return {
        title: "Hisar is not running",
        subtitle: "The extension needs the Hisar app on this computer. Open it, then try again.",
      };
    case "signed-out":
      return { title: "Sign in to Hisar", subtitle: "Sign in to your account in the Hisar app on this computer, then try again." };
    default:
      return { title: "Hisar is locked", subtitle: "Unlock Hisar to fill and save logins." };
  }
});
</script>

<template>
  <section class="panel" :data-testid="`status-${view}`">
    <div class="icon"><AppIcon :name="view === 'locked' ? 'lock' : 'laptop'" :size="20" /></div>
    <h1>{{ content.title }}</h1>
    <p class="subtitle">{{ content.subtitle }}</p>
    <p v-if="detail && detail !== content.title" class="detail muted">{{ detail }}</p>
    <button
      v-if="view === 'locked'"
      type="button"
      class="primary full-width"
      :disabled="isWaiting"
      data-testid="unlock"
      @click="emit('unlock')"
    >
      <AppIcon name="lock" /> {{ isWaiting ? "Waiting for Hisar…" : "Unlock in Hisar" }}
    </button>
    <button
      v-else-if="view === 'desktop-unavailable'"
      type="button"
      class="primary full-width"
      :disabled="isWaiting"
      data-testid="open-hisar"
      @click="emit('openHisar')"
    >
      <AppIcon name="external" /> {{ isWaiting ? "Opening Hisar…" : "Open Hisar" }}
    </button>
    <button v-else type="button" class="full-width" :disabled="isWaiting" data-testid="refresh" @click="emit('refresh')">
      <AppIcon name="refresh" /> Try again
    </button>
  </section>
</template>

<style scoped>
.icon {
  display: inline-flex;
  margin-bottom: 12px;
  padding: 9px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: var(--color-text-secondary);
}

.detail {
  margin: -8px 0 16px;
  font-size: 12px;
  overflow-wrap: anywhere;
}
</style>
