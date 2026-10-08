<script setup lang="ts">
import { computed } from "vue";
import { getInitials, getTileColor } from "../util";
import AppIcon, { type IconName } from "./AppIcon.vue";

const props = defineProps<{ title: string; size?: number; icon?: IconName }>();

const tileSize = computed(() => props.size ?? 32);
</script>

<template>
  <span
    class="tile"
    :class="{ 'icon-tile': icon }"
    :style="{
      width: `${tileSize}px`,
      height: `${tileSize}px`,
      fontSize: `${Math.round(tileSize * 0.38)}px`,
      background: icon ? undefined : getTileColor(title),
    }"
    :data-icon="icon"
    aria-hidden="true"
    ><AppIcon v-if="icon" :name="icon" :size="Math.round(tileSize * 0.5)" /><template v-else>{{ getInitials(title) }}</template></span
  >
</template>

<style scoped>
.tile {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 25%;
  color: #e9ebee;
  font-weight: 700;
  letter-spacing: 0.02em;
  box-shadow: inset 0 0 0 1px rgb(255 255 255 / 6%);
}

.icon-tile {
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
  box-shadow: inset 0 0 0 1px var(--color-border-strong);
}
</style>
