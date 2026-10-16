<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { AddressSummary } from "../../shared/protocol";
import { closePopup, describeError, sendToBackground } from "../api";
import AppIcon from "./AppIcon.vue";

// AddressList offers the addresses Hisar can fill into the page's address form: address items, then identities with an
// address. Only their city and country are known until one is filled.
const props = defineProps<{ tabId: number | undefined }>();

const allAddress = ref<AddressSummary[]>([]);
const query = ref("");
const isLoading = ref(true);
const errorMessage = ref("");
const busyItemID = ref("");

const allVisibleAddress = computed(() => {
  const normalizedQuery = query.value.trim().toLowerCase();
  if (!normalizedQuery) return allAddress.value;
  return allAddress.value.filter((address) => [address.title, address.summary, address.vaultName].some((text) => text.toLowerCase().includes(normalizedQuery)));
});

async function loadAddresses(): Promise<void> {
  try {
    allAddress.value = await sendToBackground<AddressSummary[]>({ type: "popup.listAddresses" });
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isLoading.value = false;
  }
}

async function fillAddress(address: AddressSummary): Promise<void> {
  if (props.tabId === undefined) return;
  busyItemID.value = address.itemID;
  errorMessage.value = "";
  try {
    await sendToBackground({ type: "popup.fillAddress", tabId: props.tabId, vaultID: address.vaultID, itemID: address.itemID });
    closePopup();
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    busyItemID.value = "";
  }
}

onMounted(loadAddresses);
</script>

<template>
  <div class="addresses" data-testid="address-list">
    <div v-if="allAddress.length > 0" class="search">
      <AppIcon name="search" class="search-icon" />
      <input v-model="query" type="search" placeholder="Search addresses" aria-label="Search addresses" data-testid="search-addresses" />
    </div>
    <p v-if="errorMessage" class="error" data-testid="address-error">{{ errorMessage }}</p>
    <p v-if="isLoading" class="muted empty">Looking for addresses…</p>
    <p v-else-if="allAddress.length === 0" class="muted empty" data-testid="no-addresses">No addresses in Hisar. Add one, or an address to an identity, in Hisar.</p>
    <p v-else-if="allVisibleAddress.length === 0" class="muted empty">No addresses match "{{ query }}".</p>
    <ul v-else class="list">
      <li v-for="address in allVisibleAddress" :key="address.vaultID + address.itemID" class="entry" data-testid="address-row">
        <div class="avatar" aria-hidden="true"><AppIcon :name="address.source === 'identity' ? 'identity' : 'address'" /></div>
        <div class="text">
          <p class="title-row">
            <span class="title">{{ address.title }}</span>
            <span class="tag">{{ address.vaultName }}</span>
          </p>
          <p class="summary muted">{{ [address.summary, address.source === "identity" ? "Identity" : ""].filter(Boolean).join(" · ") || "Address" }}</p>
        </div>
        <button type="button" class="primary small" :disabled="busyItemID !== '' || tabId === undefined" data-testid="fill-address" @click="fillAddress(address)">Fill</button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.search {
  position: relative;
  margin-bottom: 10px;
}

.search-icon {
  position: absolute;
  top: 50%;
  left: 10px;
  transform: translateY(-50%);
  color: var(--color-text-muted);
}

.search input {
  padding-left: 32px;
}

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
