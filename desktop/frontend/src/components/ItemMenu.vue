<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import AppIcon from "./AppIcon.vue";

defineProps<{ title: string; isFavorite: boolean; x: number; y: number }>();
const emit = defineEmits<{ close: []; toggleFavorite: [] }>();
const menu = ref<HTMLElement | null>(null);

function chooseToggleFavorite(): void {
  emit("toggleFavorite");
  emit("close");
}

function closeOnOutside(event: Event): void {
  if (event instanceof KeyboardEvent ? event.key === "Escape" : !menu.value?.contains(event.target as Node)) {
    emit("close");
  }
}

onMounted(() => {
  window.addEventListener("pointerdown", closeOnOutside);
  window.addEventListener("keydown", closeOnOutside);
  menu.value?.querySelector<HTMLButtonElement>("button")?.focus();
});
onBeforeUnmount(() => {
  window.removeEventListener("pointerdown", closeOnOutside);
  window.removeEventListener("keydown", closeOnOutside);
});
</script>

<template>
  <div ref="menu" class="card item-menu" role="menu" :aria-label="title" :style="{ left: `${x}px`, top: `${y}px` }" data-testid="item-menu">
    <button class="ghost" role="menuitem" data-testid="item-menu-favorite" @click="chooseToggleFavorite">
      <AppIcon :name="isFavorite ? 'starFilled' : 'star'" />
      {{ isFavorite ? "Remove from Favorites" : "Add to Favorites" }}
    </button>
  </div>
</template>

<style scoped>
.item-menu {
  position: fixed;
  z-index: 9;
  display: flex;
  flex-direction: column;
  min-width: 200px;
  padding: 4px;
  background: var(--color-surface-raised);
  box-shadow: 0 12px 32px var(--color-shadow);
}

.item-menu button {
  justify-content: flex-start;
  gap: 10px;
  padding: 7px 8px;
  color: var(--color-text);
}
</style>
