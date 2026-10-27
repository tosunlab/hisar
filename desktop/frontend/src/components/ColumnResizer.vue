<script setup lang="ts">
import { allColumnBound, clampColumnWidth, type ColumnID } from "../columnWidths";

// ColumnResizer sits on the right edge of the column before it, in a zero-width grid track: dragging it or pressing the
// arrow keys sets that column's width, and commit fires once the change is done, to save it.
const props = defineProps<{ column: ColumnID; label: string }>();
const width = defineModel<number>({ required: true });
const emit = defineEmits<{ commit: [] }>();
const keyboardStep = 16;

let drag: { startX: number; startWidth: number } | null = null;

function startDragging(event: PointerEvent): void {
  if (event.button !== 0) {
    return;
  }
  const handle = event.currentTarget as HTMLElement;
  // The column can be narrower than its width when the window is narrow, so the drag starts from what is on screen.
  const column = handle.previousElementSibling as HTMLElement | null;
  drag = { startX: event.clientX, startWidth: column?.getBoundingClientRect().width || width.value };
  handle.setPointerCapture?.(event.pointerId);
  event.preventDefault();
}

function moveDrag(event: PointerEvent): void {
  if (drag) {
    width.value = clampColumnWidth(props.column, drag.startWidth + event.clientX - drag.startX);
  }
}

function stopDragging(): void {
  if (drag) {
    drag = null;
    emit("commit");
  }
}

function resizeWithKey(event: KeyboardEvent): void {
  const step = event.key === "ArrowRight" ? keyboardStep : event.key === "ArrowLeft" ? -keyboardStep : 0;
  if (step === 0) {
    return;
  }
  event.preventDefault();
  width.value = clampColumnWidth(props.column, width.value + step);
  emit("commit");
}
</script>

<template>
  <div
    class="column-resizer"
    role="separator"
    aria-orientation="vertical"
    :aria-label="label"
    :aria-valuenow="width"
    :aria-valuemin="allColumnBound[column].min"
    :aria-valuemax="allColumnBound[column].max"
    tabindex="0"
    :data-testid="`resize-${column}`"
    @pointerdown="startDragging"
    @pointermove="moveDrag"
    @pointerup="stopDragging"
    @pointercancel="stopDragging"
    @keydown="resizeWithKey"
  ></div>
</template>

<style scoped>
.column-resizer {
  position: relative;
  z-index: 3;
  width: 0;
  outline: none;
  --wails-draggable: no-drag;
}

.column-resizer::before,
.column-resizer::after {
  content: "";
  position: absolute;
  top: 0;
  bottom: 0;
}

.column-resizer::before {
  left: -4px;
  width: 7px;
  cursor: col-resize;
}

.column-resizer::after {
  left: -1px;
  width: 1px;
  pointer-events: none;
}

.column-resizer:hover::after,
.column-resizer:active::after {
  background: var(--color-border-strong);
}

.column-resizer:focus-visible::after {
  left: -2px;
  width: 3px;
  background: var(--color-focus);
}
</style>
