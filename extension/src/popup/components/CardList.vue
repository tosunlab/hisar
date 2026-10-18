<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { CardSummary } from "../../shared/protocol";
import { closePopup, describeError, sendToBackground } from "../api";
import AppIcon from "./AppIcon.vue";

// CardList offers the credit cards Hisar can fill into the page's payment form. Only their brand, last four digits and
// expiry are known until one is filled.
const props = defineProps<{ tabId: number | undefined }>();

const allCard = ref<CardSummary[]>([]);
const query = ref("");
const isLoading = ref(true);
const errorMessage = ref("");
const busyItemID = ref("");

const allVisibleCard = computed(() => {
  const normalizedQuery = query.value.trim().toLowerCase();
  if (!normalizedQuery) return allCard.value;
  return allCard.value.filter((card) => [card.title, card.brand, card.last4, card.vaultName].some((text) => text.toLowerCase().includes(normalizedQuery)));
});

async function loadCards(): Promise<void> {
  try {
    allCard.value = await sendToBackground<CardSummary[]>({ type: "popup.listCards" });
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isLoading.value = false;
  }
}

async function fillCard(card: CardSummary): Promise<void> {
  if (props.tabId === undefined) return;
  busyItemID.value = card.itemID;
  errorMessage.value = "";
  try {
    await sendToBackground({ type: "popup.fillCard", tabId: props.tabId, vaultID: card.vaultID, itemID: card.itemID });
    closePopup();
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    busyItemID.value = "";
  }
}

onMounted(loadCards);
</script>

<template>
  <div class="cards" data-testid="card-list">
    <div v-if="allCard.length > 0" class="search">
      <AppIcon name="search" class="search-icon" />
      <input v-model="query" type="search" placeholder="Search cards" aria-label="Search cards" data-testid="search-cards" />
    </div>
    <p v-if="errorMessage" class="error" data-testid="card-error">{{ errorMessage }}</p>
    <p v-if="isLoading" class="muted empty">Looking for cards…</p>
    <p v-else-if="allCard.length === 0" class="muted empty" data-testid="no-cards">No credit cards in Hisar. Add one in Hisar to fill payment forms.</p>
    <p v-else-if="allVisibleCard.length === 0" class="muted empty">No cards match "{{ query }}".</p>
    <ul v-else class="list">
      <li v-for="card in allVisibleCard" :key="card.vaultID + card.itemID" class="entry" data-testid="card-row">
        <div class="avatar" aria-hidden="true"><AppIcon name="card" /></div>
        <div class="text">
          <p class="title-row">
            <span class="title">{{ card.title }}</span>
            <span class="tag">{{ card.vaultName }}</span>
          </p>
          <p class="summary muted">{{ [card.brand, card.last4 ? `•••• ${card.last4}` : "", card.expiry ? `expires ${card.expiry}` : ""].filter(Boolean).join(" · ") || "Card" }}</p>
        </div>
        <button type="button" class="primary small" :disabled="busyItemID !== '' || tabId === undefined" data-testid="fill-card" @click="fillCard(card)">Fill</button>
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
