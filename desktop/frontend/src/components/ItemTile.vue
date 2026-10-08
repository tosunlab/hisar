<script setup lang="ts">
import { computed } from "vue";
import { getInitials, getTileColor } from "../util";

const props = defineProps<{ title: string; size?: number }>();

const tileSize = computed(() => props.size ?? 32);
</script>

<template>
  <span
    class="tile"
    :style="{
      width: `${tileSize}px`,
      height: `${tileSize}px`,
      fontSize: `${Math.round(tileSize * 0.38)}px`,
      background: getTileColor(title),
    }"
    aria-hidden="true"
    >{{ getInitials(title) }}</span
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
</style>
