<script setup lang="ts">
import { computed } from "vue";
import { computePasswordStrength } from "../util";

const props = defineProps<{ password: string }>();

const strength = computed(() => computePasswordStrength(props.password));
const filledSegmentCount = computed(() => ({ empty: 0, weak: 1, fair: 2, good: 3, strong: 4 })[strength.value.level]);
</script>

<template>
  <div v-if="strength.level !== 'empty'" class="strength" :class="strength.level" data-testid="strength-meter">
    <span v-for="segment in 4" :key="segment" class="segment" :class="{ filled: segment <= filledSegmentCount }"></span>
    <span class="label">{{ strength.label }}</span>
  </div>
</template>

<style scoped>
.strength {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 8px;
}

.segment {
  width: 28px;
  height: 4px;
  border-radius: 2px;
  background: var(--color-border);
}

.weak .filled {
  background: var(--color-danger);
}

.fair .filled {
  background: var(--color-warning);
}

.good .filled,
.strong .filled {
  background: var(--color-success);
}

.label {
  margin-left: 6px;
  font-size: 12px;
  color: var(--color-text-muted);
}
</style>
