<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { PopupState } from "../shared/messages";
import type { ItemSummary } from "../shared/protocol";
import { describeError, getTargetTabId, openUpdatePage, sendToBackground } from "./api";
import AddressList from "./components/AddressList.vue";
import CardList from "./components/CardList.vue";
import IdentityList from "./components/IdentityList.vue";
import HisarLogo from "./components/HisarLogo.vue";
import ItemDetail from "./components/ItemDetail.vue";
import LoginList from "./components/LoginList.vue";
import PairingView from "./components/PairingView.vue";
import PasswordGenerator from "./components/PasswordGenerator.vue";
import SettingsView from "./components/SettingsView.vue";
import StatusView from "./components/StatusView.vue";
import VaultView from "./components/VaultView.vue";
import AppIcon from "./components/AppIcon.vue";

const props = withDefaults(defineProps<{ pollIntervalMilliseconds?: number; pollLimit?: number; statusIntervalMilliseconds?: number }>(), {
  pollIntervalMilliseconds: 1000,
  pollLimit: 120,
  statusIntervalMilliseconds: 2000,
});

const popupState = ref<PopupState>();
const tabId = ref<number>();
const isWaiting = ref(false);
const errorMessage = ref("");
const activeSection = ref<"logins" | "cards" | "addresses" | "generator">("logins");
// openedItem is the card or identity opened from its tab; choosing a tab closes it.
const openedItem = ref<ItemSummary>();
watch(activeSection, () => (openedItem.value = undefined));
const noticeMessage = ref("");
const isSettingsOpen = ref(false);
let noticeTimer: ReturnType<typeof setTimeout> | undefined;
let pollTimer: ReturnType<typeof setTimeout> | undefined;
let statusTimer: ReturnType<typeof setTimeout> | undefined;

async function refreshState(shouldReconnect = false, isOverOpenConnection = false): Promise<void> {
  try {
    popupState.value = await sendToBackground<PopupState>(
      isOverOpenConnection ? { type: "popup.getState", isOverOpenConnection } : { type: "popup.getState", shouldReconnect },
    );
    errorMessage.value = "";
  } catch (error) {
    errorMessage.value = describeError(error);
  }
}

// While the vault is shown, the popup checks Hisar's state, so locking, signing out or another account's sign-in drops
// what it shows, revealed secrets included.
function watchStatus(): void {
  statusTimer = setTimeout(async () => {
    await refreshState(false, true);
    statusTimer = undefined;
    if (popupState.value?.view === "unlocked") watchStatus();
  }, props.statusIntervalMilliseconds);
}
watch(
  () => popupState.value?.view,
  (view) => {
    if (view === "unlocked" && statusTimer === undefined) watchStatus();
  },
);
watch(
  () => popupState.value?.email,
  () => (openedItem.value = undefined),
);

function stopPolling(): void {
  if (pollTimer !== undefined) clearTimeout(pollTimer);
  pollTimer = undefined;
  isWaiting.value = false;
}

function pollUntilChanged(previousView: PopupState["view"], remaining: number): void {
  pollTimer = setTimeout(async () => {
    await refreshState();
    if (popupState.value?.view !== previousView || remaining <= 1) {
      stopPolling();
      return;
    }
    pollUntilChanged(previousView, remaining - 1);
  }, props.pollIntervalMilliseconds);
}

async function unlock(): Promise<void> {
  isWaiting.value = true;
  errorMessage.value = "";
  try {
    await sendToBackground({ type: "popup.unlock" });
    pollUntilChanged("locked", props.pollLimit);
  } catch (error) {
    errorMessage.value = describeError(error);
    isWaiting.value = false;
  }
}

const isUnlocking = ref(false);
const unlockError = ref("");

// unlockWithPassword sends the master password once; the popup keeps no copy of it.
async function unlockWithPassword(password: string): Promise<void> {
  isUnlocking.value = true;
  unlockError.value = "";
  try {
    await sendToBackground({ type: "popup.unlockWithPassword", password });
    await refreshState();
  } catch (error) {
    unlockError.value = describeError(error);
  } finally {
    isUnlocking.value = false;
  }
}

async function openHisar(): Promise<void> {
  isWaiting.value = true;
  await refreshState(true);
  isWaiting.value = false;
}

async function retry(): Promise<void> {
  isWaiting.value = true;
  await refreshState(true);
  isWaiting.value = false;
}

function showNotice(message: string): void {
  noticeMessage.value = message;
  if (noticeTimer !== undefined) clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => (noticeMessage.value = ""), 2500);
}

onMounted(async () => {
  tabId.value = await getTargetTabId().catch(() => undefined);
  await refreshState();
});

onBeforeUnmount(() => {
  stopPolling();
  if (statusTimer !== undefined) clearTimeout(statusTimer);
  if (noticeTimer !== undefined) clearTimeout(noticeTimer);
});
</script>

<template>
  <div class="popup">
    <header class="header">
      <HisarLogo show-wordmark />
      <span class="spacer"></span>
      <span v-if="popupState?.isOffline" class="tag warning" data-testid="offline">Offline</span>
      <span v-if="popupState?.view === 'unlocked' && popupState.email" class="email muted" :title="popupState.email">
        {{ popupState.email }}
      </span>
      <button
        type="button"
        class="ghost icon-button"
        :class="{ active: isSettingsOpen }"
        title="Settings"
        aria-label="Settings"
        data-testid="open-settings"
        @click="isSettingsOpen = !isSettingsOpen"
      >
        <AppIcon name="settings" />
      </button>
    </header>
    <p v-if="errorMessage && !isSettingsOpen" class="error page-error">{{ errorMessage }}</p>
    <SettingsView v-if="isSettingsOpen" @close="isSettingsOpen = false" />
    <p v-else-if="!popupState" class="muted connecting" data-testid="connecting">Connecting to Hisar…</p>
    <PairingView v-else-if="popupState.view === 'not-paired'" @paired="refreshState()" />
    <section v-else-if="popupState.view === 'unlocked'" :key="popupState.email" class="unlocked" data-testid="unlocked-view">
      <div class="segments" role="tablist">
        <button
          type="button"
          role="tab"
          :class="{ active: activeSection === 'logins' }"
          :aria-selected="activeSection === 'logins'"
          data-testid="section-logins"
          @click="activeSection = 'logins'"
        >
          {{ popupState.isVaultAvailable ? "Vault" : "Logins" }}
        </button>
        <button
          v-if="popupState.isCardFillAvailable"
          type="button"
          role="tab"
          :class="{ active: activeSection === 'cards' }"
          :aria-selected="activeSection === 'cards'"
          data-testid="section-cards"
          @click="activeSection = 'cards'"
        >
          Cards
        </button>
        <button
          v-if="popupState.isAddressFillAvailable || popupState.isIdentityFillAvailable"
          type="button"
          role="tab"
          :class="{ active: activeSection === 'addresses' }"
          :aria-selected="activeSection === 'addresses'"
          data-testid="section-addresses"
          @click="activeSection = 'addresses'"
        >
          {{ popupState.isIdentityFillAvailable ? "Identities" : "Addresses" }}
        </button>
        <button
          type="button"
          role="tab"
          :class="{ active: activeSection === 'generator' }"
          :aria-selected="activeSection === 'generator'"
          data-testid="section-generator"
          @click="activeSection = 'generator'"
        >
          Generator
        </button>
      </div>
      <VaultView
        v-if="activeSection === 'logins' && popupState.isVaultAvailable"
        :tab-id="tabId"
        :can-create="popupState.isCreateAvailable"
        :can-fill-cards="popupState.isCardFillAvailable"
        :can-fill-identities="popupState.isIdentityFillAvailable"
        @notice="showNotice"
      />
      <LoginList v-else-if="activeSection === 'logins'" :tab-id="tabId" @notice="showNotice" />
      <ItemDetail
        v-else-if="openedItem"
        :summary="openedItem"
        :tab-id="tabId"
        :can-fill="tabId !== undefined"
        @back="openedItem = undefined"
        @notice="showNotice"
      />
      <CardList v-else-if="activeSection === 'cards'" :tab-id="tabId" :can-open="popupState.isVaultAvailable" @open="openedItem = $event" />
      <template v-else-if="activeSection === 'addresses'">
        <template v-if="popupState.isIdentityFillAvailable">
          <h2 class="group-title muted">Identities</h2>
          <IdentityList :tab-id="tabId" :can-open="popupState.isVaultAvailable" @open="openedItem = $event" />
          <h2 v-if="popupState.isAddressFillAvailable" class="group-title muted">Addresses</h2>
        </template>
        <AddressList v-if="popupState.isAddressFillAvailable" :tab-id="tabId" />
      </template>
      <PasswordGenerator v-else :tab-id="tabId" @notice="showNotice" />
    </section>
    <StatusView
      v-else
      :view="popupState.view"
      :detail="popupState.errorMessage"
      :update-required="popupState.updateRequired"
      :is-waiting="isWaiting"
      :is-password-unlock-available="popupState.isPasswordUnlockAvailable"
      :is-unlocking="isUnlocking"
      :unlock-error="unlockError"
      @unlock="unlock"
      @unlock-with-password="unlockWithPassword"
      @open-hisar="openHisar"
      @refresh="retry"
      @open-update-page="openUpdatePage"
    />
    <p v-if="noticeMessage" class="notice" role="status" data-testid="notice">{{ noticeMessage }}</p>
  </div>
</template>

<style scoped>
.popup {
  position: relative;
  min-height: 200px;
  background:
    radial-gradient(600px 240px at 50% -40%, var(--color-glow) 0%, transparent 70%),
    var(--color-background);
}

.header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border);
}

.header .icon-button.active {
  background: var(--color-selected);
  color: var(--color-text);
}

.email {
  max-width: 170px;
  overflow: hidden;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.page-error {
  margin: 12px 16px 0;
}

.connecting {
  margin: 0;
  padding: 40px 16px;
  text-align: center;
}

.unlocked {
  padding: 12px 12px 16px;
}

.segments {
  display: flex;
  gap: 2px;
  margin-bottom: 12px;
  padding: 2px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-panel);
}

.segments button {
  flex: 1;
  padding: 5px 10px;
  border: none;
  border-radius: var(--radius-small);
  background: transparent;
  color: var(--color-text-secondary);
}

.segments button.active {
  background: var(--color-selected);
  color: var(--color-text);
}

.group-title {
  margin: 4px 8px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.notice {
  position: fixed;
  bottom: 12px;
  left: 50%;
  margin: 0;
  padding: 7px 12px;
  transform: translateX(-50%);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface-raised);
  box-shadow: 0 8px 24px rgb(0 0 0 / 25%);
  font-size: 12px;
  white-space: nowrap;
}
</style>
