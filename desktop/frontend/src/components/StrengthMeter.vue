<script setup lang="ts">
import { computed } from "vue";
import { usePasswordStrength } from "../passwordStrength";

const props = defineProps<{ password: string }>();

const segmentCountByLevel: Record<string, number> = { weak: 1, fair: 2, good: 3, strong: 4 };
const strength = usePasswordStrength(() => props.password);
const filledSegmentCount = computed(() => segmentCountByLevel[strength.value.level] ?? 0);
</script>

<template>
  <div v-if="strength.level !== ''" class="strength" :class="strength.level" data-testid="strength-meter">
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
  font-size: var(--font-size-12);
  color: var(--color-text-muted);
}
</style>
