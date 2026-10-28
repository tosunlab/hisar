<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { Events } from "@wailsio/runtime";
import { api, type MissingAgentKey } from "../api";
import { openSettings } from "../store";

const notices = ref<MissingAgentKey[]>([]);
let generation = 0;
let removeListeners: (() => void)[] = [];

async function reload(): Promise<void> {
  const current = ++generation;
  try {
    const pending = await api.listMissingAgentKeys();
    if (current === generation) notices.value = pending;
  } catch {
    return;
  }
}

onMounted(() => {
  removeListeners = [
    Events.On("missing-agent-keys-changed", reload),
    Events.On("remote-change", reload),
    Events.On("agent-sessions-changed", reload),
  ];
  window.addEventListener("focus", reload);
  reload();
});

onBeforeUnmount(() => {
  generation++;
  removeListeners.forEach((remove) => remove());
  window.removeEventListener("focus", reload);
});
</script>

<template>
  <aside v-if="notices.length" class="missing-agent-keys" aria-label="Coding agent SSH keys" data-testid="missing-agent-key-notices">
    <section v-for="notice in notices" :key="notice.sessionID" role="status">
      <strong>{{ notice.agentName }} asked for an SSH key.</strong>
      <p>No SSH key is in a vault available to coding agents.</p>
      <button @click="openSettings('developer')">Open Developer tools</button>
    </section>
  </aside>
</template>

<style scoped>
.missing-agent-keys {
  position: fixed;
  top: 64px;
  right: 16px;
  z-index: 90;
  width: min(340px, calc(100vw - 32px));
  max-height: 35vh;
  overflow-y: auto;
  border: 1px solid var(--color-border);
  border-radius: 10px;
  background: var(--color-panel);
  box-shadow: 0 4px 16px var(--color-shadow);
}

section {
  padding: 16px;
}

section + section {
  border-top: 1px solid var(--color-border);
}

p {
  margin: 8px 0;
}
</style>
