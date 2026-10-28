<script setup lang="ts">
import { ref, useId } from "vue";
import { useModalFocus } from "../modalFocus";
import AppIcon, { type IconName } from "./AppIcon.vue";

const props = withDefaults(defineProps<{ title: string; subtitle?: string; icon: IconName; isDanger?: boolean; isClosable?: boolean }>(), {
  isDanger: false,
  isClosable: true,
});
const emit = defineEmits<{ close: [] }>();

const panel = ref<HTMLElement | null>(null);
const headingID = useId();
const subtitleID = useId();
useModalFocus(panel, () => { if (props.isClosable) emit("close"); });
</script>

<template>
  <div class="overlay" @click.self="isClosable && emit('close')">
    <div ref="panel" class="panel" role="dialog" aria-modal="true" :aria-labelledby="headingID" :aria-describedby="subtitleID" :data-dialog-safe-action="isDanger || undefined" tabindex="-1">
      <div class="panel-header">
        <span class="panel-icon" :class="{ danger: isDanger }"><AppIcon :name="icon" /></span>
        <div class="panel-heading">
          <h2 :id="headingID">{{ title }}</h2>
          <p v-if="subtitle" :id="subtitleID" class="muted">{{ subtitle }}</p>
        </div>
      </div>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: var(--color-backdrop);
}

.panel {
  width: min(520px, 100%);
  max-height: 100%;
  padding: 20px 22px;
  overflow-y: auto;
  border-radius: var(--radius-large);
  background: var(--color-panel);
  box-shadow: 0 24px 64px var(--color-shadow);
  text-align: left;
}

.panel-header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 16px;
}

.panel-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  flex: none;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-large);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.panel-icon.danger {
  border-color: var(--color-danger-border);
  background: var(--color-danger-background);
  color: var(--color-danger-text);
}

.panel-heading {
  flex: 1;
  min-width: 0;
}

h2 {
  margin: 0;
  font-size: var(--font-size-15);
  font-weight: 600;
}

.panel-heading p {
  margin: 3px 0 0;
  font-size: var(--font-size-12);
}

:slotted(.consequences) {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0 0 14px;
  padding: 10px 12px 10px 28px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: var(--font-size-12);
  line-height: 1.45;
}

:slotted(.consequences.warning) {
  border-color: var(--color-warning-border);
  background: var(--color-warning-background);
  color: var(--color-warning-text);
}

:slotted(.actions) {
  justify-content: flex-end;
  margin-top: 6px;
}

:slotted(label) {
  display: block;
  margin-bottom: 12px;
}

:slotted(label > span) {
  display: block;
  margin-bottom: 5px;
  font-size: var(--font-size-12);
  font-weight: 500;
  color: var(--color-text-secondary);
}

:slotted(label input) {
  width: 100%;
}

:slotted(.hint) {
  margin: 6px 0 0;
  font-size: var(--font-size-11);
}

:slotted(.check-row) {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--font-size-13);
  cursor: pointer;
}

:slotted(.check-row input) {
  width: auto;
}
</style>
