<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import AppIcon, { type IconName } from "./AppIcon.vue";

// ContextMenuAction is one item of a right-click menu; null in allAction draws a separator.
export interface ContextMenuAction {
  id: string;
  label: string;
  icon: IconName;
  isDanger?: boolean;
  run: () => void;
}

const props = defineProps<{ label: string; x: number; y: number; allAction: (ContextMenuAction | null)[] }>();
const emit = defineEmits<{ close: [] }>();
const menu = ref<HTMLElement | null>(null);
// position keeps the menu inside the window: it opens at the pointer and moves back when it would pass an edge.
const position = ref({ left: props.x, top: props.y });

function choose(action: ContextMenuAction): void {
  emit("close");
  action.run();
}

function listButtons(): HTMLButtonElement[] {
  return [...(menu.value?.querySelectorAll<HTMLButtonElement>("button") ?? [])];
}

function moveFocus(step: number): void {
  const allButton = listButtons();
  const index = allButton.indexOf(document.activeElement as HTMLButtonElement);
  allButton[(index + step + allButton.length) % allButton.length]?.focus();
}

function handleKey(event: KeyboardEvent): void {
  if (event.key === "Escape") {
    emit("close");
  } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    event.preventDefault();
    moveFocus(event.key === "ArrowDown" ? 1 : -1);
  }
}

function closeOnOutside(event: Event): void {
  if (!menu.value?.contains(event.target as Node)) {
    emit("close");
  }
}

function fitInWindow(): void {
  const bounds = menu.value?.getBoundingClientRect();
  if (!bounds) {
    return;
  }
  const margin = 8;
  position.value = {
    left: Math.max(margin, Math.min(props.x, window.innerWidth - bounds.width - margin)),
    top: Math.max(margin, Math.min(props.y, window.innerHeight - bounds.height - margin)),
  };
}

// Actions that load later, such as an item's copy actions, change the menu's size.
watch(
  () => props.allAction.length,
  async () => {
    await nextTick();
    fitInWindow();
  },
);

onMounted(() => {
  window.addEventListener("pointerdown", closeOnOutside);
  window.addEventListener("keydown", handleKey);
  fitInWindow();
  listButtons()[0]?.focus();
});
onBeforeUnmount(() => {
  window.removeEventListener("pointerdown", closeOnOutside);
  window.removeEventListener("keydown", handleKey);
});
</script>

<template>
  <div
    ref="menu"
    class="card context-menu"
    role="menu"
    :aria-label="label"
    :style="{ left: `${position.left}px`, top: `${position.top}px` }"
    data-testid="context-menu"
    @contextmenu.prevent
  >
    <template v-for="(action, index) in allAction" :key="action?.id ?? `separator-${index}`">
      <hr v-if="action === null" class="separator" />
      <button
        v-else
        class="ghost"
        :class="{ danger: action.isDanger }"
        role="menuitem"
        :data-testid="`context-menu-${action.id}`"
        @click="choose(action)"
      >
        <AppIcon :name="action.icon" />
        {{ action.label }}
      </button>
    </template>
  </div>
</template>

<style scoped>
.context-menu {
  position: fixed;
  z-index: 9;
  display: flex;
  flex-direction: column;
  min-width: 200px;
  max-width: 320px;
  padding: 4px;
  background: var(--color-surface-raised);
  box-shadow: 0 12px 32px var(--color-shadow);
}

.context-menu button {
  justify-content: flex-start;
  gap: 10px;
  padding: 7px 8px;
  color: var(--color-text);
  text-align: left;
}

.context-menu button.danger {
  color: var(--color-danger-text);
}

.separator {
  width: 100%;
  margin: 4px 0;
  border: none;
  border-top: 1px solid var(--color-border);
}
</style>
