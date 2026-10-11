<script setup lang="ts">
import { getInitials, getTileColor } from "../util";

defineProps<{ name: string; size?: number }>();
</script>

<template>
  <span
    class="member-avatar"
    :style="{ width: `${size ?? 32}px`, height: `${size ?? 32}px`, fontSize: `${Math.round((size ?? 32) * 0.36)}px`, ...getTileColor(name) }"
    aria-hidden="true"
    >{{ getInitials(name) }}</span
  >
</template>

<style scoped>
.member-avatar {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  font-weight: 650;
  letter-spacing: 0.02em;
  box-shadow: inset 0 0 0 1px var(--color-tile-edge);
}
</style>
