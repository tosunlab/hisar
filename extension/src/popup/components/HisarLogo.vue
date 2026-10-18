<script setup lang="ts">
defineProps<{ size?: number; showWordmark?: boolean }>();
</script>

<template>
  <span class="logo">
    <svg :width="size ?? 22" :height="size ?? 22" viewBox="0 0 32 32" aria-hidden="true">
      <rect width="32" height="32" rx="8" fill="var(--color-primary)" />
      <path d="M8 24.5V10.5h3V7.5h2.5v3h5V7.5H21v3h3v14h-6v-5a2 2 0 0 0-4 0v5z" fill="var(--color-primary-text)" />
    </svg>
    <span v-if="showWordmark" class="wordmark">Hisar</span>
  </span>
</template>

<style scoped>
.logo {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.wordmark {
  font-size: 15px;
  font-weight: 700;
  letter-spacing: -0.01em;
}
</style>
