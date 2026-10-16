<script setup lang="ts">
const allPath = {
  lock: "M7 10V7a5 5 0 0 1 10 0v3M5 10h14v10H5z",
  search: "M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM20 20l-4-4",
  plus: "M12 5v14M5 12h14",
  copy: "M9 9h10v10H9zM5 15V5h10",
  eye: "M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z",
  eyeOff: "M3 3l18 18M10.6 5.1A10 10 0 0 1 12 5c6 0 10 7 10 7a17 17 0 0 1-3.2 3.9M6.6 6.6C3.9 8.3 2 12 2 12s4 7 10 7a9.7 9.7 0 0 0 5.4-1.6M9.9 9.9a3 3 0 0 0 4.2 4.2",
  edit: "M4 20h4L19 9l-4-4L4 16zM14 6l4 4",
  trash: "M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13",
  key: "M15 9a4 4 0 1 1-3.5 6L5 21H3v-3l1-1h2v-2h2l1.5-1.5A4 4 0 0 1 15 9zM16 8h.01",
  passkey: "M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM2 21a7 7 0 0 1 11-5.7M18 12a3 3 0 1 0 0 6 3 3 0 0 0 0-6zM18 18v4M18 20h2",
  globe: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18",
  user: "M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0",
  users: "M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM2 21a7 7 0 0 1 14 0M16 3.5a4 4 0 0 1 0 7M18 14a7 7 0 0 1 4 7",
  vault: "M4 4h16v16H4zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM12 9V7M12 17v-2M4 8h2M4 16h2",
  shield: "M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z",
  refresh: "M20 11a8 8 0 0 0-14.9-3M4 4v4h4M4 13a8 8 0 0 0 14.9 3M20 20v-4h-4",
  external: "M14 4h6v6M20 4l-9 9M18 14v6H4V6h6",
  check: "M5 12l5 5L20 7",
  note: "M6 3h9l4 4v14H6zM9 12h6M9 16h6",
  card: "M3 6h18v12H3zM3 10h18M7 15h3",
  identity: "M3 5h18v14H3zM9 8.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM5.5 16.5c.6-1.6 1.9-2.3 3.5-2.3s2.9.7 3.5 2.3M15 10h4M15 14h4",
  field: "M4 6h16M4 12h16M4 18h10",
  close: "M6 6l12 12M18 6L6 18",
  signOut: "M15 4h4v16h-4M10 8l-4 4 4 4M6 12h11",
  fingerprint: "M12 11v3a8 8 0 0 0 1.5 4.7M8.5 8.8A4 4 0 0 1 16 11v1.5M8 12v1.5a11 11 0 0 0 1.3 5.2M5.3 15a14 14 0 0 1-.3-3 7 7 0 0 1 11.6-5.3M19 12v.5a15 15 0 0 1-.5 3.9M15.8 15.6a12 12 0 0 1-.8 3.6",
  settings: "M4 7h9M17 7h3M4 17h3M11 17h9M15 9a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM9 19a2 2 0 1 0 0-4 2 2 0 0 0 0 4z",
  terminal: "M4 5h16v14H4zM8 10l3 2.5L8 15M13 15h3",
  chevronDown: "M6 9l6 6 6-6",
  chevronRight: "M9 6l6 6-6 6",
  laptop: "M5 6h14v10H5zM2 19h20",
  phone: "M8 3h8v18H8zM11 18h2",
  mail: "M3 6h18v12H3zM3 7l9 6 9-6",
  clock: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 7v5l3 2",
  warning: "M12 4l9 16H3zM12 10v4M12 17h.01",
  offline: "M3 3l18 18M8.5 8.6A5 5 0 0 0 7 18h10M14 6.3A5.5 5.5 0 0 1 18 11a3.5 3.5 0 0 1 2.4 5.9",
  import: "M12 4v11M7.5 10.5L12 15l4.5-4.5M5 20h14",
  file: "M6 3h8l4 4v14H6zM14 3v4h4",
  help: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .9-1 1.6v.4M12 17h.01",
  export: "M12 15V4M7.5 8.5L12 4l4.5 4.5M5 20h14",
  more: "M5 12h.01M12 12h.01M19 12h.01",
  link: "M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1",
} as const;

export type IconName = keyof typeof allPath;

defineProps<{ name: IconName; size?: number }>();
</script>

<template>
  <svg
    class="icon"
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

<style scoped>
.icon {
  flex: none;
}
</style>
