<script setup lang="ts">
import { copyText } from "../store";
import AppIcon from "./AppIcon.vue";

defineProps<{ secretKey: string; testId?: string }>();
</script>

<template>
  <section class="secret-key-block">
    <h2>Your Secret Key</h2>
    <div class="key" :data-testid="testId ?? 'secret-key'" :aria-label="secretKey">
      <span v-for="(part, index) in secretKey.split('-')" :key="index" class="key-part monospace">{{ part }}</span>
    </div>
    <button @click="copyText(secretKey, 'Secret Key')"><AppIcon name="copy" /> Copy</button>
  </section>
</template>

<style scoped>
.secret-key-block { display: flex; flex-direction: column; align-items: center; gap: 12px; }
h2 { margin: 0; font-size: var(--font-size-15); }
.key {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 4px;
  padding: 18px 12px;
  border: 1px dashed var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  user-select: text;
  -webkit-user-select: text;
}
.key-part {
  padding: 4px 6px;
  border-radius: var(--radius-small);
  background: var(--color-surface-raised);
  font-size: var(--font-size-15);
}
</style>
