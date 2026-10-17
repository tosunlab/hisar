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
  apiKey: "M15 7a3 3 0 1 1 0 6 3 3 0 0 1 0-6zM12.9 11.1 7 17M9 15l2 2M3 4h2v16H3M21 4h-2v16h2",
  address: "M12 21s-7-6.1-7-11.5a7 7 0 0 1 14 0C19 14.9 12 21 12 21zM12 7.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4z",
  server: "M4 4h16v6H4zM4 14h16v6H4zM8 7h.01M8 17h.01M12 7h5M12 17h5",
  database: "M12 3c4.4 0 8 1.3 8 3v12c0 1.7-3.6 3-8 3s-8-1.3-8-3V6c0-1.7 3.6-3 8-3zM4 6c0 1.7 3.6 3 8 3s8-1.3 8-3M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3",
  license: "M14 20H5V3h10l4 4v5M9 8h5M9 11h6M9 14h3M18 13a3 3 0 1 1 0 6 3 3 0 0 1 0-6zM16.5 18.5 16 22l2-1 2 1-.5-3.5",
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
  tag: "M4 4h7l9 9-7 7-9-9zM8.5 8.5h.01",
  star: "M12 3.5l2.6 5.3 5.9.9-4.25 4.1 1 5.8L12 16.9l-5.25 2.7 1-5.8L3.5 9.7l5.9-.9z",
  starFilled: "M12 3.5l2.6 5.3 5.9.9-4.25 4.1 1 5.8L12 16.9l-5.25 2.7 1-5.8L3.5 9.7l5.9-.9z",
  link: "M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1",
  building: "M4 21V4h10v17M14 9h6v12M2 21h20M7.5 8h3M7.5 12h3M7.5 16h3M17 13h.01M17 17h.01",
  duplicate: "M9 9h11v11H9zM5 15V4h11M14.5 12v5M12 14.5h5",
  move: "M3 12h13M12 7l5 5-5 5M20 4v16",
  // Preset item icons (core/item IconPresets); the others reuse the icons above.
  bank: "M3 10l9-6 9 6M5 10v8M9.5 10v8M14.5 10v8M19 10v8M3 20h18",
  wallet: "M4 7h15a1 1 0 0 1 1 1v11H4zM4 7V5h12v2M15 13h2",
  crypto: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM9.5 8h3.5a2 2 0 0 1 0 4H9.5zM9.5 12H14a2 2 0 0 1 0 4H9.5zM9.5 8v8M11 6.5V8M11 16v1.5",
  cart: "M3 4h2l2.4 11h10.2L20 8H6.2M9 20h.01M17 20h.01",
  gift: "M4 10h16v10H4zM3 7h18v3H3zM12 7v13M12 7C10.5 4 7 4 7 6s3.5 1 5 1zM12 7c1.5-3 5-3 5-1s-3.5 1-5 1z",
  chat: "M4 5h16v11H9l-5 4z",
  social: "M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM16 12v1.5a2.5 2.5 0 0 0 5 0V12a9 9 0 1 0-3.5 7.1",
  work: "M3 8h18v12H3zM8 8V5h8v3M3 13h18",
  home: "M3 11l9-7 9 7M5 9.5V20h14V9.5M10 20v-6h4v6",
  school: "M2 9l10-5 10 5-10 5zM6 11v5c0 1.5 2.7 3 6 3s6-1.5 6-3v-5M22 9v5",
  cloud: "M7 18h10a4 4 0 0 0 .5-8A6 6 0 0 0 6 9a4.5 4.5 0 0 0 1 9z",
  code: "M8 7l-5 5 5 5M16 7l5 5-5 5M14 4l-4 16",
  wifi: "M2 9a15 15 0 0 1 20 0M5 12.5a10 10 0 0 1 14 0M8.5 16a5 5 0 0 1 7 0M12 19.5h.01",
  game: "M6 8h12a4 4 0 0 1 3.9 4.9l-.9 3.6a2.5 2.5 0 0 1-4.4 1L15 16H9l-1.6 1.5a2.5 2.5 0 0 1-4.4-1L2.1 12.9A4 4 0 0 1 6 8zM6.5 12h3M8 10.5v3M15 12h.01M17 11h.01",
  music: "M9 18V5l11-2v13M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0zM20 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0z",
  video: "M3 6h13v12H3zM16 10l5-3v10l-5-3",
  health: "M3 12h4l2-5 4 10 2-5h6",
  travel: "M21 16v-2l-8-5V3.5a1.5 1.5 0 0 0-3 0V9l-8 5v2l8-2.5V19l-2 1.5V22l3.5-1 3.5 1v-1.5L13 19v-5.5z",
  car: "M5 11l1.5-4.5A2 2 0 0 1 8.4 5h7.2a2 2 0 0 1 1.9 1.5L19 11M3 11h18v6H3zM6 17v2M18 17v2M7 14h.01M17 14h.01",
  heart: "M12 20s-8-4.6-8-10.5A4.5 4.5 0 0 1 12 7a4.5 4.5 0 0 1 8 2.5C20 15.4 12 20 12 20z",
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
    :fill="name === 'starFilled' ? 'currentColor' : 'none'"
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
