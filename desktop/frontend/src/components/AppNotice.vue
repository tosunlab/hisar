<script setup lang="ts">
import { store } from "../store";
import AppIcon from "./AppIcon.vue";
</script>

<template>
  <div v-if="store.noticeMessage" class="notice" :class="{ 'notice-error': store.noticeKind === 'error' }" :role="store.noticeKind === 'error' ? 'alert' : 'status'" data-testid="notice">
    <AppIcon :name="store.noticeKind === 'error' ? 'warning' : 'check'" /> {{ store.noticeMessage }}
  </div>
</template>
