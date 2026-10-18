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
  settings: "M4 7h9M17 7h3M4 17h3M11 17h9M15 9a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM9 19a2 2 0 1 0 0-4 2 2 0 0 0 0 4z",
  globe: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18",
  address: "M12 21s-7-6.1-7-11.5a7 7 0 0 1 14 0C19 14.9 12 21 12 21zM12 7.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4z",
  identity: "M3 5h18v14H3zM9 8.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM5.5 16.5c.6-1.6 1.9-2.3 3.5-2.3s2.9.7 3.5 2.3M15 10h4M15 14h4",
  card: "M3 6h18v12H3zM3 10h18M7 15h4",
  note: "M6 3h9l4 4v14H6zM14 3v5h5M9 13h6M9 17h6",
  server: "M4 4h16v6H4zM4 14h16v6H4zM8 7h.01M8 17h.01",
  database: "M12 3c4.4 0 8 1.3 8 3v12c0 1.7-3.6 3-8 3s-8-1.3-8-3V6c0-1.7 3.6-3 8-3zM4 6c0 1.7 3.6 3 8 3s8-1.3 8-3M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3",
  terminal: "M3 5h18v14H3zM7 10l3 2-3 2M12 15h5",
  license: "M5 3h14v18H5zM9 8h6M9 12h6M9 16h3",
  star: "M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3l-5.6 2.9 1.1-6.2L3 9.6l6.2-.9z",
  eye: "M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12zM12 9a3 3 0 1 1 0 6 3 3 0 0 1 0-6z",
  "eye-off": "M3 3l18 18M10.6 5.1A10.6 10.6 0 0 1 12 5c6.4 0 10 7 10 7a17 17 0 0 1-3.2 4M6.6 6.6A17 17 0 0 0 2 12s3.6 7 10 7a9.7 9.7 0 0 0 5.4-1.6M9.9 9.9a3 3 0 0 0 4.2 4.2",
  back: "M15 18l-6-6 6-6",
  plus: "M12 5v14M5 12h14",
} as const;

export type IconName = keyof typeof allPath;

defineProps<{ name: IconName; size?: number; isFilled?: boolean }>();
</script>

<template>
  <svg
    :width="size ?? 16"
    :height="size ?? 16"
    viewBox="0 0 24 24"
    :fill="isFilled ? 'currentColor' : 'none'"
    stroke="currentColor"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="allPath[name]" />
  </svg>
</template>
