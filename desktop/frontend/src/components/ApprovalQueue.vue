<script setup lang="ts">
defineProps<{
  requests: { requestID: string; requester: string; purpose: string; seconds: number }[];
}>();
const emit = defineEmits<{ deny: [requestID: string]; resize: [] }>();
</script>

<template>
  <details v-if="requests.length" class="queued-requests" data-testid="approval-queue" @toggle="emit('resize')" @click.stop>
    <summary>{{ requests.length }} more waiting</summary>
    <ul>
      <li v-for="pending in requests" :key="pending.requestID">
        <div>
          <strong>{{ pending.requester }}</strong>
          <p>{{ pending.purpose }}</p>
          <p class="muted">{{ pending.seconds > 0 ? `Expires in ${pending.seconds} seconds` : "Expired" }}</p>
        </div>
        <button :aria-label="`Deny request from ${pending.requester}`" :disabled="pending.seconds === 0" @click="emit('deny', pending.requestID)">Deny</button>
      </li>
    </ul>
  </details>
</template>

<style scoped>
.queued-requests {
  flex: none;
  padding: 12px 0;
  border-top: 1px solid var(--color-border);
  font-size: var(--font-size-12);
}

summary {
  cursor: pointer;
}

ul {
  list-style: none;
  padding: 0;
  max-height: min(180px, 30vh);
  overflow-y: auto;
}

li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
}

p {
  margin: 4px 0;
  overflow-wrap: anywhere;
}
</style>
