<script setup lang="ts">
import { onMounted, ref } from "vue";
import { auth, getErrorMessage, getServer, type AdminServer } from "../api";

const server = ref<AdminServer | null>(null);
const errorMessage = ref("");

onMounted(async () => {
  try {
    server.value = await getServer();
  } catch (error) {
    errorMessage.value = getErrorMessage(error);
  }
});
</script>

<template>
  <section>
    <h1>Overview</h1>
    <p v-if="errorMessage" class="error" role="alert">{{ errorMessage }}</p>
    <dl class="facts">
      <div>
        <dt>Signed in as</dt>
        <dd data-testid="admin-name">{{ auth.session?.name }}</dd>
      </div>
      <div>
        <dt>Server</dt>
        <dd data-testid="public-url">{{ server?.publicUrl ?? "…" }}</dd>
      </div>
    </dl>
  </section>
</template>

<style scoped>
h1 {
  margin: 0 0 20px;
  font-size: 22px;
  font-weight: 650;
}

.error {
  max-width: 640px;
  margin-bottom: 16px;
}

.facts {
  display: grid;
  gap: 1px;
  max-width: 640px;
  margin: 0;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-large);
  background: var(--color-border);
}

.facts div {
  display: grid;
  grid-template-columns: 160px 1fr;
  gap: 16px;
  padding: 12px 16px;
  background: var(--color-surface);
}

dt {
  color: var(--color-text-muted);
}

dd {
  margin: 0;
  overflow-wrap: anywhere;
}
</style>
