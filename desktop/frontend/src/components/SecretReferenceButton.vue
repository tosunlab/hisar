<script setup lang="ts">
import { copySecretReference } from "../store";
import AppIcon from "./AppIcon.vue";

defineProps<{ fieldName: string; label: string }>();
</script>

<template>
  <button
    class="ghost icon-button"
    :aria-label="`Copy secret reference for ${label}`"
    title="Copy secret reference — use it in a terminal with hisar run or hisar read"
    data-testid="copy-secret-reference"
    @click="copySecretReference(fieldName)"
  >
    <AppIcon name="terminal" />
  </button>
</template>
