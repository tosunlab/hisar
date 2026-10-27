<script setup lang="ts">
import { ref } from "vue";
import AppIcon, { type IconName } from "./AppIcon.vue";

// SidebarNotice is a time-critical notice above the vaults: one line that discloses the whole sentence on a click,
// so a long one never pushes the vaults down. The slot holds actions such as Dismiss.
defineProps<{ icon: IconName; text: string; isWarning?: boolean }>();
const isExpanded = ref(false);
</script>

<template>
  <div class="sidebar-notice" :class="{ warning: isWarning }" role="status">
    <button class="notice-toggle" :aria-expanded="isExpanded" @click="isExpanded = !isExpanded">
      <AppIcon :name="icon" :size="14" class="notice-icon" />
      <span class="notice-text" :class="{ expanded: isExpanded }">{{ text }}</span>
      <AppIcon name="chevronRight" :size="12" class="notice-chevron" :class="{ expanded: isExpanded }" />
    </button>
    <slot />
  </div>
</template>

<style scoped>
.sidebar-notice {
  display: flex;
  align-items: flex-start;
  gap: 2px;
  margin-bottom: 4px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.notice-toggle {
  display: flex;
  flex: 1;
  align-items: flex-start;
  justify-content: flex-start;
  gap: 8px;
  min-width: 0;
  padding: 6px 8px;
  border: none;
  background: none;
  font-size: var(--font-size-12);
  line-height: 1.4;
  color: var(--color-text-secondary);
  text-align: left;
}

.notice-toggle:hover:not(:disabled) {
  border: none;
  background: none;
  color: var(--color-text);
}

.notice-icon,
.notice-chevron {
  flex: none;
  margin-top: 1px;
  color: var(--color-text-muted);
}

.warning .notice-icon {
  color: var(--color-warning);
}

.notice-chevron.expanded {
  transform: rotate(90deg);
}

.notice-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notice-text.expanded {
  overflow-wrap: anywhere;
  white-space: normal;
}

.sidebar-notice :slotted(button) {
  flex: none;
  margin: 3px 3px 0 0;
  padding: 3px;
}
</style>
