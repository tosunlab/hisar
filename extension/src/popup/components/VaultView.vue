<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { ItemQuery } from "../../shared/messages";
import type { ItemSummary, LoginSummary } from "../../shared/protocol";
import { describeError, sendToBackground } from "../api";
import { writeToClipboard } from "../clipboard";
import AppIcon from "./AppIcon.vue";
import ItemDetail from "./ItemDetail.vue";
import ItemIcon from "./ItemIcon.vue";
import LoginList from "./LoginList.vue";
import NewItemView from "./NewItemView.vue";

// canCreate: the app saves new items from the browser, so the view offers New item.
// canFillCards and canFillIdentities: the app fills those, so an opened card or identity offers Fill on this page.
const props = withDefaults(
  defineProps<{ tabId: number | undefined; canCreate?: boolean; canFillCards?: boolean; canFillIdentities?: boolean; searchDelayMilliseconds?: number }>(),
  {
    canCreate: false,
    canFillCards: false,
    canFillIdentities: false,
    searchDelayMilliseconds: 150,
  },
);

// isFillable is an item Fill can put on the page: a login saved for the tab, or a card or an identity.
function isFillable(item: ItemSummary): boolean {
  if (allMatchingKey.value.has(getKey(item))) return true;
  if (props.tabId === undefined) return false;
  return (item.category === "credit-card" && props.canFillCards) || (item.category === "identity" && props.canFillIdentities);
}
const emit = defineEmits<{ notice: [message: string] }>();

type CategoryChoice = "all" | "favorites" | "login" | "credit-card" | "identity" | "secure-note";

const allCategoryChoice: { id: CategoryChoice; label: string }[] = [
  { id: "all", label: "All" },
  { id: "favorites", label: "Favorites" },
  { id: "login", label: "Logins" },
  { id: "credit-card", label: "Cards" },
  { id: "identity", label: "Identities" },
  { id: "secure-note", label: "Notes" },
];

const query = ref("");
const category = ref<CategoryChoice>("all");
const allItem = ref<ItemSummary[]>([]);
const allMatchingKey = ref(new Set<string>());
const isLoading = ref(true);
const errorMessage = ref("");
const selected = ref<ItemSummary>();
const isCreating = ref(false);
const pageUrl = ref("");
const busyItemID = ref("");
const searchInput = ref<HTMLInputElement>();
let searchTimer: ReturnType<typeof setTimeout> | undefined;
let loadNumber = 0;

// The suggestions for this tab are shown above the vault until the user searches another category.
const isShowingSuggestions = computed(() => category.value === "all" || category.value === "login");

const allLastUsed = ref<Record<string, number>>({});

const allListedItem = computed(() => {
  // A login already suggested for this tab is not listed twice while nothing is searched; search results put the items
  // filled most recently first.
  if (query.value.trim() === "") {
    return isShowingSuggestions.value ? allItem.value.filter((item) => !allMatchingKey.value.has(getKey(item))) : allItem.value;
  }
  const lastUsed = (item: ItemSummary): number => allLastUsed.value[getKey(item)] ?? 0;
  return [...allItem.value].sort((first, second) => lastUsed(second) - lastUsed(first));
});

function getKey(item: Pick<ItemSummary, "vaultID" | "itemID">): string {
  return `${item.vaultID}/${item.itemID}`;
}

function buildQuery(): ItemQuery {
  const itemQuery: ItemQuery = {};
  if (query.value.trim()) itemQuery.query = query.value.trim();
  if (category.value === "favorites") itemQuery.isFavoriteOnly = true;
  else if (category.value !== "all") itemQuery.category = category.value;
  return itemQuery;
}

async function loadItems(): Promise<void> {
  const number = ++loadNumber;
  try {
    const result = await sendToBackground<ItemSummary[]>({ type: "popup.listItems", query: buildQuery() });
    if (number !== loadNumber) return;
    allItem.value = result;
    errorMessage.value = "";
  } catch (error) {
    if (number === loadNumber) errorMessage.value = describeError(error);
  } finally {
    if (number === loadNumber) isLoading.value = false;
  }
}

function scheduleLoad(): void {
  if (searchTimer !== undefined) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => void loadItems(), props.searchDelayMilliseconds);
}

watch(query, scheduleLoad);
watch(category, () => {
  isLoading.value = true;
  void loadItems();
});

function rememberMatches(allLogin: LoginSummary[], url: string): void {
  allMatchingKey.value = new Set(allLogin.map(getKey));
  pageUrl.value = url;
}

function showCreated(item: ItemSummary): void {
  isCreating.value = false;
  selected.value = item;
  emit("notice", "Saved to Hisar");
  void loadItems();
}

function openLogin(login: LoginSummary, url: string): void {
  selected.value = {
    vaultID: login.vaultID,
    itemID: login.itemID,
    category: "login",
    title: login.title,
    subtitle: login.username,
    vaultName: login.vaultName,
    url,
    isFavorite: allItem.value.find((item) => getKey(item) === getKey(login))?.isFavorite ?? false,
    hasOneTimePassword: login.hasOneTimePassword ?? false,
  };
}

async function closeCreate(): Promise<void> {
  isCreating.value = false;
  await nextTick();
  searchInput.value?.focus();
}

async function closeDetail(): Promise<void> {
  selected.value = undefined;
  await nextTick();
  searchInput.value?.focus();
}

function markFavorite(isFavorite: boolean): void {
  if (!selected.value) return;
  const key = getKey(selected.value);
  selected.value = { ...selected.value, isFavorite };
  allItem.value = allItem.value
    .map((item) => (getKey(item) === key ? { ...item, isFavorite } : item))
    .filter((item) => category.value !== "favorites" || item.isFavorite);
}

async function copyPassword(item: ItemSummary): Promise<void> {
  busyItemID.value = item.itemID;
  errorMessage.value = "";
  try {
    await writeToClipboard(await sendToBackground<string>({ type: "popup.revealField", vaultID: item.vaultID, itemID: item.itemID, fieldID: "password" }));
    emit("notice", "Password copied");
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    busyItemID.value = "";
  }
}

// Keyboard: the arrow keys move between the search box and the rows, Enter opens a row, and Ctrl or Cmd+C on a login
// row copies its password.
function moveFocus(event: KeyboardEvent): void {
  const container = event.currentTarget as HTMLElement;
  const allRow = Array.from(container.querySelectorAll<HTMLElement>("[data-row]"));
  const active = document.activeElement as HTMLElement | null;
  const index = active ? allRow.indexOf(active) : -1;
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "c" && index >= 0) {
    const copyButton = allRow[index].closest("li")?.querySelector<HTMLButtonElement>("[data-copy]");
    if (copyButton && !window.getSelection()?.toString()) {
      event.preventDefault();
      copyButton.click();
    }
    return;
  }
  if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
  event.preventDefault();
  if (event.key === "ArrowDown") allRow[index + 1]?.focus();
  else if (index <= 0) searchInput.value?.focus();
  else allRow[index - 1].focus();
}

onMounted(() => {
  void loadItems();
  searchInput.value?.focus();
  sendToBackground<Record<string, number> | null>({ type: "popup.getAllLastUsed" })
    .then((result) => (allLastUsed.value = result ?? {}))
    .catch(() => undefined);
});
onBeforeUnmount(() => {
  if (searchTimer !== undefined) clearTimeout(searchTimer);
});
</script>

<template>
  <NewItemView v-if="isCreating" :page-url="pageUrl" @cancel="closeCreate" @created="showCreated" />
  <ItemDetail
    v-else-if="selected"
    :summary="selected"
    :tab-id="tabId"
    :can-fill="isFillable(selected)"
    @back="closeDetail"
    @notice="emit('notice', $event)"
    @favorite="markFavorite"
  />
  <div v-show="!selected && !isCreating" class="vault" data-testid="vault-view" @keydown="moveFocus">
    <div class="search-row">
      <div class="search">
        <AppIcon name="search" class="search-icon" />
        <input ref="searchInput" v-model="query" type="search" placeholder="Search all items" aria-label="Search all items" data-testid="vault-search" />
      </div>
      <button v-if="canCreate" type="button" class="icon-button" title="New item" aria-label="New item" data-testid="new-item-open" @click="isCreating = true">
        <AppIcon name="plus" />
      </button>
    </div>
    <div class="chips" role="tablist" aria-label="Categories">
      <button
        v-for="choice in allCategoryChoice"
        :key="choice.id"
        type="button"
        role="tab"
        class="chip"
        :class="{ active: category === choice.id }"
        :aria-selected="category === choice.id"
        :data-testid="`category-${choice.id}`"
        @click="category = choice.id"
      >
        {{ choice.label }}
      </button>
    </div>
    <template v-if="isShowingSuggestions">
      <h2 class="group-title muted">Suggested for this page</h2>
      <LoginList :tab-id="tabId" :query="query" @notice="emit('notice', $event)" @loaded="rememberMatches" @open="openLogin" />
    </template>
    <h2 class="group-title muted">{{ query.trim() ? "Search results" : category === "all" ? "All items" : allCategoryChoice.find((choice) => choice.id === category)?.label }}</h2>
    <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    <p v-if="isLoading" class="muted empty">Loading your items…</p>
    <p v-else-if="allListedItem.length === 0" class="muted empty" data-testid="no-items">
      {{ query.trim() ? `Nothing matches "${query.trim()}".` : category === "favorites" ? "Star an item to see it here." : "No items here yet." }}
    </p>
    <ul v-else class="list">
      <li v-for="item in allListedItem" :key="getKey(item)" class="entry" data-testid="item-row">
        <div class="row-button" role="button" tabindex="0" data-row data-testid="open-item" @click="selected = item" @keydown.enter="selected = item">
          <ItemIcon :category="item.category" :url="item.url" />
          <div class="text">
            <p class="title-row">
              <span class="title">{{ item.title }}</span>
              <AppIcon v-if="item.isFavorite" name="star" class="star" :size="12" is-filled />
              <span class="tag">{{ item.vaultName }}</span>
            </p>
            <p class="subtitle muted">{{ item.subtitle || " " }}</p>
          </div>
        </div>
        <button
          v-if="item.category === 'login'"
          type="button"
          class="ghost icon-button"
          title="Copy password"
          aria-label="Copy password"
          :disabled="busyItemID !== ''"
          data-testid="item-copy-password"
          data-copy
          @click="copyPassword(item)"
        >
          <AppIcon name="key" />
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.search-row {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.search {
  position: relative;
  flex: 1;
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

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 10px;
}

.chip {
  flex: none;
  padding: 3px 10px;
  border-radius: 12px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 12px;
}

.chip.active {
  border-color: var(--color-primary);
  background: var(--color-primary);
  color: var(--color-primary-text);
}

.group-title {
  margin: 4px 8px 4px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.empty {
  margin: 14px 0;
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

.entry:hover,
.entry:focus-within {
  background: var(--color-hover);
}

.row-button {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 10px;
  min-width: 0;
  cursor: pointer;
}

.row-button:focus-visible,
.vault :deep([data-row]:focus-visible) {
  outline: none;
}

.entry:has([data-row]:focus-visible),
.vault :deep(.entry:has([data-row]:focus-visible)) {
  box-shadow: inset 0 0 0 1px var(--color-focus);
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

.title-row .tag,
.star {
  flex: none;
}

.star {
  color: var(--color-warning);
}

.title,
.subtitle {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title {
  font-weight: 500;
}

.subtitle {
  font-size: 12px;
}
</style>
