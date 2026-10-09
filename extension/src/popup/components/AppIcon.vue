<script setup lang="ts">
const allPath = {
  lock: "M7 10V7a5 5 0 0 1 10 0v3M5 10h14v10H5z",
  search: "M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM20 20l-4-4",
  copy: "M9 9h10v10H9zM5 15V5h10",
  key: "M15 9a4 4 0 1 1-3.5 6L5 21H3v-3l1-1h2v-2h2l1.5-1.5A4 4 0 0 1 15 9zM16 8h.01",
  user: "M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0",
  refresh: "M20 11a8 8 0 0 0-14.9-3M4 4v4h4M4 13a8 8 0 0 0 14.9 3M20 20v-4h-4",
  external: "M14 4h6v6M20 4l-9 9M18 14v6H4V6h6",
  check: "M5 12l5 5L20 7",
  laptop: "M5 6h14v10H5zM2 19h20",
  offline: "M3 3l18 18M8.5 8.6A5 5 0 0 0 7 18h10M14 6.3A5.5 5.5 0 0 1 18 11a3.5 3.5 0 0 1 2.4 5.9",
  fill: "M4 12h11M11 8l4 4-4 4M19 4v16",
  globe: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18",
} as const;

export type IconName = keyof typeof allPath;

defineProps<{ name: IconName; size?: number }>();
</script>

<template>
  <svg
    :width="size ?? 16"
    :height="size ?? 16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="allPath[name]" />
  </svg>
</template>
