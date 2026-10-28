<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import AppIcon, { type IconName } from "./AppIcon.vue";
import { useMenu, type MenuCloseReason } from "../menu";

// ContextMenuAction is one item of a right-click menu; null in allAction draws a separator.
export interface ContextMenuAction {
  id: string;
  label: string;
  icon: IconName;
  isDanger?: boolean;
  shortcut?: string;
  disabled?: boolean;
  opensDestination?: boolean;
  run: () => void;
}

const props = withDefaults(
  defineProps<{ label: string; x: number; y: number; allAction: (ContextMenuAction | null)[]; anchor?: HTMLElement | null; invoker?: HTMLElement | null; testId?: string; actionTestIdPrefix?: string }>(),
  { testId: "context-menu" },
);
const emit = defineEmits<{ close: [reason: MenuCloseReason] }>();
const menu = ref<HTMLElement | null>(null);
// position keeps the menu inside the window: it opens at the pointer and moves back when it would pass an edge.
const position = ref({ left: props.x, top: props.y });
let resizeObserver: ResizeObserver | undefined;
const initialInvoker = document.activeElement instanceof HTMLElement ? document.activeElement : null;
const dismiss = useMenu(menu, (reason) => emit("close", reason), () => props.invoker ?? initialInvoker);

function choose(action: ContextMenuAction, event: MouseEvent): void {
  dismiss(!action.opensDestination && event.detail === 0 ? "action" : "destination");
  action.run();
}

function fitInWindow(): void {
  const bounds = menu.value?.getBoundingClientRect();
  if (!bounds) {
    return;
  }
  const margin = 8;
  const anchor = props.anchor?.getBoundingClientRect();
  const left = anchor ? anchor.right - bounds.width : props.x;
  const below = anchor ? anchor.bottom + 6 : props.y;
  const top = anchor && below + bounds.height > window.innerHeight - margin ? anchor.top - bounds.height - 6 : below;
  position.value = {
    left: Math.max(margin, Math.min(left, window.innerWidth - bounds.width - margin)),
    top: Math.max(margin, Math.min(top, window.innerHeight - bounds.height - margin)),
  };
}

// Actions that load later, such as an item's copy actions, change the menu's size.
watch(
  () => [props.x, props.y, props.allAction, props.anchor],
  async () => {
    await nextTick();
    fitInWindow();
  },
);

onMounted(() => {
  window.addEventListener("resize", fitInWindow);
  if (typeof ResizeObserver !== "undefined") {
    resizeObserver = new ResizeObserver(fitInWindow);
    if (menu.value) resizeObserver.observe(menu.value);
    if (props.anchor) {
      resizeObserver.observe(props.anchor);
      if (props.anchor.parentElement) resizeObserver.observe(props.anchor.parentElement);
    }
  }
  fitInWindow();
});
onBeforeUnmount(() => {
  window.removeEventListener("resize", fitInWindow);
  resizeObserver?.disconnect();
});
</script>

<template>
  <div
    ref="menu"
    class="card context-menu"
    role="menu"
    :aria-label="label"
    :style="{ left: `${position.left}px`, top: `${position.top}px` }"
    @pointerdown.stop
    :data-testid="testId"
    @contextmenu.prevent
  >
    <template v-for="(action, index) in allAction" :key="action?.id ?? `separator-${index}`">
      <hr v-if="action === null" class="separator" />
      <button
        v-else
        class="ghost"
        :class="{ danger: action.isDanger }"
        role="menuitem"
        :disabled="action.disabled"
        tabindex="-1"
        :data-testid="`${actionTestIdPrefix ?? testId}-${action.id}`"
        @click="choose(action, $event)"
      >
        <AppIcon :name="action.icon" />
        {{ action.label }}
        <kbd v-if="action.shortcut">{{ action.shortcut }}</kbd>
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
  max-height: calc(100dvh - 16px);
  overflow-y: auto;
  box-sizing: border-box;
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
  flex-shrink: 0;
}

.context-menu button.danger {
  color: var(--color-danger-text);
}

.separator {
  width: 100%;
  margin: 4px 0;
  border: none;
  border-top: 1px solid var(--color-border);
  flex-shrink: 0;
}

.context-menu kbd {
  margin-left: auto;
}
</style>
