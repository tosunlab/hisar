<script setup lang="ts">
import { onMounted, ref } from "vue";
import type { IdentitySummary } from "../../shared/protocol";
import { closePopup, describeError, sendToBackground } from "../api";
import AppIcon from "./AppIcon.vue";

// IdentityList offers the identities Hisar can fill into the page: name, email, phone and birth date, and the address
// form beside them. Only their name and email are known until one is filled.
const props = defineProps<{ tabId: number | undefined }>();

const allIdentity = ref<IdentitySummary[]>([]);
const isLoading = ref(true);
const errorMessage = ref("");
const busyItemID = ref("");

async function loadIdentities(): Promise<void> {
  try {
    allIdentity.value = await sendToBackground<IdentitySummary[]>({ type: "popup.listIdentities" });
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isLoading.value = false;
  }
}

async function fillIdentity(identity: IdentitySummary): Promise<void> {
  if (props.tabId === undefined) return;
  busyItemID.value = identity.itemID;
  errorMessage.value = "";
  try {
    await sendToBackground({ type: "popup.fillIdentity", tabId: props.tabId, vaultID: identity.vaultID, itemID: identity.itemID });
    closePopup();
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    busyItemID.value = "";
  }
}

onMounted(loadIdentities);
</script>

<template>
  <div class="identities" data-testid="identity-list">
    <p v-if="errorMessage" class="error" data-testid="identity-error">{{ errorMessage }}</p>
    <p v-if="isLoading" class="muted empty">Looking for identities…</p>
    <p v-else-if="allIdentity.length === 0" class="muted empty" data-testid="no-identities">No identities in Hisar.</p>
    <ul v-else class="list">
      <li v-for="identity in allIdentity" :key="identity.vaultID + identity.itemID" class="entry" data-testid="identity-row">
        <div class="avatar" aria-hidden="true"><AppIcon name="identity" /></div>
        <div class="text">
          <p class="title-row">
            <span class="title">{{ identity.title }}</span>
            <span class="tag">{{ identity.vaultName }}</span>
          </p>
          <p class="summary muted">{{ identity.summary || "Identity" }}</p>
        </div>
        <button type="button" class="primary small" :disabled="busyItemID !== '' || tabId === undefined" data-testid="fill-identity" @click="fillIdentity(identity)">Fill</button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.empty {
  margin: 18px 0;
  text-align: center;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.entry {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 8px;
  border-radius: var(--radius);
}

.entry:hover {
  background: var(--color-hover);
}

.avatar {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  margin-right: 4px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
}

.text {
  flex: 1;
  min-width: 0;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
}

.title-row .tag {
  flex: none;
}

.title,
.summary {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title {
  font-weight: 500;
}

.summary {
  font-size: 12px;
}
</style>
