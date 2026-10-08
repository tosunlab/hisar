<script setup lang="ts">
import { computed } from "vue";
import { respondSSHApproval, store } from "../store";
import AppIcon from "./AppIcon.vue";

const approval = computed(() => store.allPendingApproval[0]);
const destination = computed(() => approval.value?.destinationHost || approval.value?.destinationFingerprint || "");
</script>

<template>
  <div v-if="approval" class="overlay">
    <div class="card approval" role="alertdialog" aria-modal="true" aria-labelledby="approval-title" aria-describedby="approval-details">
      <span class="approval-icon"><AppIcon name="key" :size="22" /></span>
      <h2 id="approval-title">
        <span class="monospace">{{ approval.clientName }}</span> wants to use <strong>{{ approval.keyTitle }}</strong>
      </h2>
      <dl id="approval-details" class="approval-details">
        <div v-if="destination">
          <dt>Destination</dt>
          <dd :class="{ monospace: !approval.destinationHost }" data-testid="approval-destination">{{ destination }}</dd>
        </div>
        <div v-if="approval.clientPath">
          <dt>Program</dt>
          <dd class="monospace">{{ approval.clientPath }}</dd>
        </div>
        <div>
          <dt>Key</dt>
          <dd class="monospace">{{ approval.fingerprint }}</dd>
        </div>
      </dl>
      <p v-if="approval.isForwarded" class="warning">This request comes through a forwarded agent on another machine.</p>
      <p class="muted note">Allowing lets this program use the key until Hisar locks.</p>
      <div class="row actions">
        <span v-if="store.allPendingApproval.length > 1" class="muted">{{ store.allPendingApproval.length - 1 }} more waiting</span>
        <div class="spacer"></div>
        <button data-testid="deny-approval" @click="respondSSHApproval(approval.requestID, false)">Deny</button>
        <button class="primary" data-testid="allow-approval" @click="respondSSHApproval(approval.requestID, true)">Allow</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgb(0 0 0 / 55%);
}

.approval {
  width: min(420px, 100%);
  padding: 22px;
  background: var(--color-panel);
  box-shadow: 0 24px 64px rgb(0 0 0 / 50%);
}

.approval-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-large);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

h2 {
  margin: 14px 0 14px;
  font-size: 16px;
  font-weight: 600;
  line-height: 1.4;
}

.approval-details {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0 0 14px;
  padding: 12px 14px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
}

.approval-details div {
  display: grid;
  grid-template-columns: 84px 1fr;
  gap: 8px;
}

.approval-details dt {
  font-size: 12px;
  color: var(--color-text-muted);
}

.approval-details dd {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: 12px;
  user-select: text;
  -webkit-user-select: text;
}

.warning {
  margin: 0 0 10px;
  font-size: 12px;
  color: var(--color-warning);
}

.note {
  margin: 0 0 16px;
  font-size: 12px;
}
</style>
