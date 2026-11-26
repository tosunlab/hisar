<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { VisibilityGuard } from "../shared/click-guard";
import type { InlineMenuBroadcast, InlineMenuState, OtherLogin } from "../shared/messages";
import type { AddressSummary, CardSummary, IdentitySummary, LoginSummary } from "../shared/protocol";
import { sendToBackground } from "./api";

const props = withDefaults(
  defineProps<{
    sessionID: string;
    isVisibilityTracked?: boolean;
    pollIntervalMilliseconds?: number;
    pollLimit?: number;
    searchDelayMilliseconds?: number;
    now?: () => number;
    isEventTrusted?: (event: Event) => boolean;
  }>(),
  {
    isVisibilityTracked: false,
    pollIntervalMilliseconds: 1000,
    pollLimit: 120,
    searchDelayMilliseconds: 150,
    now: () => performance.now(),
    isEventTrusted: (event: Event) => event.isTrusted,
  },
);

type MenuAction =
  | { kind: "login"; login: LoginSummary }
  | { kind: "code"; login: LoginSummary }
  | { kind: "address"; address: AddressSummary }
  | { kind: "card"; card: CardSummary }
  | { kind: "identity"; identity: IdentitySummary }
  | { kind: "more" }
  | { kind: "expand" }
  | { kind: "other"; login: OtherLogin }
  | { kind: "confirm-other"; login: OtherLogin }
  | { kind: "suggest" }
  | { kind: "pair" }
  | { kind: "unlock" }
  | { kind: "open-hisar" };

const menuState = ref<InlineMenuState>();
const selectedIndex = ref(-1);
const errorMessage = ref("");
// isRunning is true while a fill runs; the menu takes no other choice then, so one click fills once.
const isRunning = ref(false);
const isWaiting = ref(false);
const root = ref<HTMLElement>();
const guard = new VisibilityGuard();
let observer: IntersectionObserver | undefined;
let pollTimer: ReturnType<typeof setTimeout> | undefined;
let lastReportedHeight = 0;

// The menu shows the first collapsedCount entries and a Show all row; Show all expands it in place, with a filter, to
// every entry, so a long list never sends the user to another window or tab.
const collapsedCount = 5;
const isExpanded = ref(false);
const filterText = ref("");
const filterInput = ref<HTMLInputElement>();

const isOneTimeCodeMenu = computed(() => menuState.value?.purpose === "one-time-code");

// While the expanded menu is filtered, it also searches the user's other logins. Choosing one asks first, in the menu,
// whether to add this page's site to it; only a click on that question fills it.
const canFillOtherLogins = computed(() => menuState.value?.view === "logins" && menuState.value.canFillOtherLogins === true);
const allOtherLogin = ref<OtherLogin[]>([]);
const pendingOther = ref<OtherLogin>();
const minimumSearchLength = 2;
let searchTimer: ReturnType<typeof setTimeout> | undefined;
let searchNumber = 0;

type FillAction = Extract<MenuAction, { kind: "login" | "code" | "address" | "card" | "identity" }>;

function describeCard(card: CardSummary): string {
  return [card.brand, card.last4 ? `•••• ${card.last4}` : "", card.expiry ? `expires ${card.expiry}` : ""].filter(Boolean).join(" · ") || "Card";
}

function describeEntry(action: FillAction): { title: string; subtitle: string } {
  switch (action.kind) {
    case "login":
      return { title: action.login.title, subtitle: action.login.username || "No username" };
    case "code":
      return { title: action.login.title, subtitle: `One-time code · ${action.login.username || "no username"}` };
    case "card":
      return { title: action.card.title, subtitle: describeCard(action.card) };
    case "identity":
      return { title: action.identity.title, subtitle: action.identity.summary || "Identity" };
    case "address":
      return {
        title: action.address.title,
        subtitle: [action.address.summary, action.address.source === "identity" ? "Identity" : ""].filter(Boolean).join(" · ") || "Address",
      };
  }
}

function getEntryKey(action: FillAction): string {
  const summary =
    action.kind === "login" || action.kind === "code" ? action.login : action.kind === "card" ? action.card : action.kind === "identity" ? action.identity : action.address;
  return `${summary.vaultID}/${summary.itemID}`;
}

// allEntry is every fill the menu offers on this field, in the order the background sent them.
const allEntry = computed<FillAction[]>(() => {
  const state = menuState.value;
  if (state?.view !== "logins") return [];
  switch (state.purpose) {
    case "new-password":
      return [];
    case "identity":
      return (state.allIdentity ?? []).map((identity) => ({ kind: "identity", identity }));
    case "card":
      return (state.allCard ?? []).map((card) => ({ kind: "card", card }));
    case "address":
      return (state.allAddress ?? []).map((address) => ({ kind: "address", address }));
    default:
      return state.allLogin.map((login) => ({ kind: state.purpose === "one-time-code" ? "code" : "login", login }));
  }
});

const shownEntry = computed<FillAction[]>(() => {
  if (!isExpanded.value) return allEntry.value.slice(0, collapsedCount);
  const allWord = filterText.value.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (allWord.length === 0) return allEntry.value;
  return allEntry.value.filter((action) => {
    const { title, subtitle } = describeEntry(action);
    const text = `${title} ${subtitle}`.toLowerCase();
    return allWord.every((word) => text.includes(word));
  });
});

const hiddenCount = computed(() => (isExpanded.value ? 0 : allEntry.value.length - shownEntry.value.length));
// The Show all row also opens the search of the user's other logins, however few are saved for this page.
const isExpandOffered = computed(() => !isExpanded.value && (hiddenCount.value > 0 || canFillOtherLogins.value));
const shownOtherLogin = computed(() => (isExpanded.value ? allOtherLogin.value : []));

const entryNoun = computed(() => {
  switch (menuState.value?.purpose) {
    case "card":
      return "cards";
    case "identity":
      return "identities";
    case "address":
      return "addresses";
    case "one-time-code":
      return "codes";
    default:
      return "logins";
  }
});

const allAction = computed<MenuAction[]>(() => {
  const state = menuState.value;
  if (!state) return [];
  switch (state.view) {
    case "logins":
      if (state.purpose === "new-password") return state.suggestedPassword ? [{ kind: "suggest" }] : [];
      if (pendingOther.value) return [];
      return [
        ...shownEntry.value,
        ...shownOtherLogin.value.map((login): MenuAction => ({ kind: "other", login })),
        ...(isExpandOffered.value ? [{ kind: "expand" } as MenuAction] : []),
      ];
    case "not-paired":
      return [{ kind: "pair" }];
    case "locked":
      return [{ kind: "unlock" }];
    case "desktop-unavailable":
      return [{ kind: "open-hisar" }];
    case "update-required":
      return [{ kind: "more" }];
    default:
      return [];
  }
});

// A choice made before the menu has been fully visible for a moment is refused, and never carried out later: the page
// may have covered the menu while it was made. The menu says so instead of ignoring it.
const notYetVisibleMessage = "Hisar is making sure this menu is visible. Choose again";

function isChoiceAllowed(): boolean {
  if (!props.isVisibilityTracked || guard.isInteractionAllowed(props.now())) return true;
  errorMessage.value = notYetVisibleMessage;
  return false;
}

async function loadState(): Promise<void> {
  try {
    menuState.value = await sendToBackground<InlineMenuState>({ type: "inlineMenu.getState", sessionID: props.sessionID });
    errorMessage.value = menuState.value.view === "logins" ? (menuState.value.errorMessage ?? "") : "";
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error);
  }
}

function stopPolling(): void {
  if (pollTimer !== undefined) clearTimeout(pollTimer);
  pollTimer = undefined;
  isWaiting.value = false;
}

function pollUntilUnlocked(remaining: number): void {
  pollTimer = setTimeout(async () => {
    await loadState();
    if (menuState.value?.view !== "locked" || remaining <= 1) {
      stopPolling();
      return;
    }
    pollUntilUnlocked(remaining - 1);
  }, props.pollIntervalMilliseconds);
}

// expand shows every entry and the filter; the selection moves to the first entry that was hidden.
async function expand(): Promise<void> {
  const firstRevealed = shownEntry.value.length;
  isExpanded.value = true;
  await nextTick();
  selectedIndex.value = firstRevealed;
  filterInput.value?.focus({ preventScroll: true });
}

async function runAction(action: MenuAction): Promise<void> {
  if (isRunning.value) return;
  if (action.kind === "expand") {
    await expand();
    return;
  }
  if (action.kind === "other") {
    pendingOther.value = action.login;
    return;
  }
  errorMessage.value = "";
  isRunning.value = true;
  try {
    switch (action.kind) {
      case "login":
        await sendToBackground({ type: "inlineMenu.fillLogin", sessionID: props.sessionID, vaultID: action.login.vaultID, itemID: action.login.itemID });
        return;
      case "code":
        await sendToBackground({ type: "inlineMenu.fillOneTimeCode", sessionID: props.sessionID, vaultID: action.login.vaultID, itemID: action.login.itemID });
        return;
      case "address":
        await sendToBackground({ type: "inlineMenu.fillAddress", sessionID: props.sessionID, vaultID: action.address.vaultID, itemID: action.address.itemID });
        return;
      case "identity":
        await sendToBackground({ type: "inlineMenu.fillIdentity", sessionID: props.sessionID, vaultID: action.identity.vaultID, itemID: action.identity.itemID });
        return;
      case "card":
        await sendToBackground({ type: "inlineMenu.fillCard", sessionID: props.sessionID, vaultID: action.card.vaultID, itemID: action.card.itemID });
        return;
      case "suggest":
        await sendToBackground({ type: "inlineMenu.useSuggestedPassword", sessionID: props.sessionID });
        return;
      case "confirm-other":
        await sendToBackground({ type: "inlineMenu.fillOtherLogin", sessionID: props.sessionID, vaultID: action.login.vaultID, itemID: action.login.itemID });
        return;
      case "more":
      case "pair":
        await sendToBackground({ type: "inlineMenu.openPopup", sessionID: props.sessionID });
        return;
      case "unlock":
        isWaiting.value = true;
        await sendToBackground({ type: "inlineMenu.unlock", sessionID: props.sessionID });
        pollUntilUnlocked(props.pollLimit);
        return;
      case "open-hisar":
        isWaiting.value = true;
        menuState.value = await sendToBackground<InlineMenuState>({ type: "inlineMenu.openHisar", sessionID: props.sessionID });
        isWaiting.value = false;
        return;
    }
  } catch (error) {
    isWaiting.value = false;
    errorMessage.value = error instanceof Error ? error.message : String(error);
  } finally {
    isRunning.value = false;
  }
}

function chooseAction(action: MenuAction, event: Event): void {
  if (!props.isEventTrusted(event) || !isChoiceAllowed()) return;
  void runAction(action);
}

function pressKey(key: InlineMenuBroadcast["key"]): void {
  const count = allAction.value.length;
  if (count === 0) return;
  if (key === "ArrowDown") selectedIndex.value = (selectedIndex.value + 1) % count;
  else if (key === "ArrowUp") selectedIndex.value = selectedIndex.value <= 0 ? count - 1 : selectedIndex.value - 1;
  else if (key === "Enter" && selectedIndex.value >= 0) {
    if (!isChoiceAllowed()) return;
    void runAction(allAction.value[selectedIndex.value]);
  }
}

function handleBroadcast(message: InlineMenuBroadcast): void {
  if (message?.target !== "inline-menu" || message.sessionID !== props.sessionID) return;
  if (message.type === "inlineMenu.pressKey") pressKey(message.key);
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape" && pendingOther.value) pendingOther.value = undefined;
  else if (event.key === "Escape") void sendToBackground({ type: "inlineMenu.close", sessionID: props.sessionID }).catch(() => undefined);
  else if (event.key === "ArrowDown" || event.key === "ArrowUp" || event.key === "Enter") {
    event.preventDefault();
    pressKey(event.key);
  }
}

async function reportHeight(): Promise<void> {
  await nextTick();
  // The whole content's height, borders included, even while the frame is shorter and the menu scrolls.
  const element = root.value;
  const height = element ? Math.ceil(element.scrollHeight + element.offsetHeight - element.clientHeight) : 0;
  if (height <= 0 || height === lastReportedHeight) return;
  lastReportedHeight = height;
  await sendToBackground({ type: "inlineMenu.resize", sessionID: props.sessionID, height }).catch(() => undefined);
}

function trackVisibility(): void {
  if (!props.isVisibilityTracked || !root.value) return;
  observer = new IntersectionObserver(
    (allEntry) => {
      for (const entry of allEntry) {
        const isVisible = (entry as IntersectionObserverEntry & { isVisible?: boolean }).isVisible === true;
        guard.recordVisibility(isVisible, props.now());
      }
    },
    { threshold: [1], trackVisibility: true, delay: 100 } as IntersectionObserverInit,
  );
  observer.observe(root.value);
}

watch([menuState, errorMessage, isExpanded, filterText, allOtherLogin, pendingOther], reportHeight);
watch(menuState, () => {
  selectedIndex.value = -1;
  pendingOther.value = undefined;
});
watch(filterText, (text) => {
  if (searchTimer !== undefined) clearTimeout(searchTimer);
  const query = text.trim();
  const number = ++searchNumber;
  if (!canFillOtherLogins.value || query.length < minimumSearchLength) {
    allOtherLogin.value = [];
    return;
  }
  searchTimer = setTimeout(async () => {
    try {
      const result = await sendToBackground<OtherLogin[]>({ type: "inlineMenu.searchOtherLogins", sessionID: props.sessionID, query });
      if (number === searchNumber) allOtherLogin.value = result ?? [];
    } catch {
      if (number === searchNumber) allOtherLogin.value = [];
    }
  }, props.searchDelayMilliseconds);
});
// Typing in the filter selects the first match, so Enter fills it.
watch(filterText, () => (selectedIndex.value = shownEntry.value.length > 0 ? 0 : -1));
watch(selectedIndex, async () => {
  await nextTick();
  root.value?.querySelector(".selected")?.scrollIntoView({ block: "nearest" });
});

onMounted(async () => {
  chrome.runtime.onMessage.addListener(handleBroadcast);
  window.addEventListener("keydown", handleKeydown);
  trackVisibility();
  await loadState();
});

onBeforeUnmount(() => {
  stopPolling();
  if (searchTimer !== undefined) clearTimeout(searchTimer);
  observer?.disconnect();
  chrome.runtime.onMessage.removeListener(handleBroadcast);
  window.removeEventListener("keydown", handleKeydown);
});

defineExpose({ pressKey });
</script>

<template>
  <div ref="root" class="menu" :class="{ busy: isRunning }" role="listbox" aria-label="Hisar" :aria-busy="isRunning">
    <p v-if="errorMessage" class="message error" data-testid="inline-error">{{ errorMessage }}</p>
    <p v-if="!menuState && !errorMessage" class="message muted">Loading…</p>
    <template v-else-if="menuState?.view === 'logins' && menuState.purpose === 'new-password'">
      <button
        v-if="menuState.suggestedPassword"
        type="button"
        class="item"
        :class="{ selected: selectedIndex === 0 }"
        data-testid="suggested-password"
        @click="chooseAction({ kind: 'suggest' }, $event)"
      >
        <span class="avatar key" aria-hidden="true">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M15 9a4 4 0 1 1-3.5 6L5 21H3v-3l1-1h2v-2h2l1.5-1.5A4 4 0 0 1 15 9zM16 8h.01" /></svg>
        </span>
        <span class="text">
          <span class="title">Use suggested password</span>
          <span class="subtitle monospace">{{ menuState.suggestedPassword }}</span>
        </span>
      </button>
    </template>
    <template v-else-if="menuState?.view === 'logins' && pendingOther">
      <div class="confirm" data-testid="inline-confirm">
        <p class="title">Use “{{ pendingOther.title }}” on {{ menuState.host }}?</p>
        <p class="muted">
          This login is saved for {{ pendingOther.host || "another website" }}. Hisar adds {{ menuState.host }} to it and fills it here. Continue only if
          you trust this website.
        </p>
        <button type="button" class="item action" data-testid="inline-confirm-other" @click="chooseAction({ kind: 'confirm-other', login: pendingOther }, $event)">
          Add {{ menuState.host }} and fill
        </button>
        <button type="button" class="item cancel" data-testid="inline-cancel-other" @click="pendingOther = undefined">Cancel</button>
      </div>
    </template>
    <template v-else-if="menuState?.view === 'logins'">
      <div v-if="isExpanded" class="filter">
        <svg class="filter-icon" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M20 20l-4-4" /></svg>
        <input
          ref="filterInput"
          v-model="filterText"
          type="search"
          :placeholder="allEntry.length === 0 ? 'Search all logins' : canFillOtherLogins ? `Filter ${allEntry.length} ${entryNoun} or search all` : `Filter ${allEntry.length} ${entryNoun}`"
          :aria-label="`Filter ${entryNoun}`"
          autocomplete="off"
          spellcheck="false"
          data-testid="inline-filter"
        />
      </div>
      <button
        v-for="(action, index) in shownEntry"
        :key="getEntryKey(action)"
        type="button"
        class="item"
        :class="{ selected: selectedIndex === index }"
        :data-testid="`inline-${action.kind}`"
        @click="chooseAction(action, $event)"
      >
        <span v-if="action.kind === 'login'" class="avatar" aria-hidden="true">{{ (action.login.title || "?").slice(0, 1).toUpperCase() }}</span>
        <span v-else class="avatar key" aria-hidden="true">
          <svg v-if="action.kind === 'code'" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="13" r="8" /><path d="M12 9v4l2.5 2.5M10 2h4" /></svg>
          <svg v-else-if="action.kind === 'card'" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18v12H3zM3 10h18M7 15h4" /></svg>
          <svg v-else-if="action.kind === 'identity' || action.address.source === 'identity'" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 5h18v14H3zM9 8.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM5.5 16.5c.6-1.6 1.9-2.3 3.5-2.3s2.9.7 3.5 2.3M15 10h4M15 14h4" /></svg>
          <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 21s-7-6.1-7-11.5a7 7 0 0 1 14 0C19 14.9 12 21 12 21zM12 7.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4z" /></svg>
        </span>
        <span class="text">
          <span class="title">{{ describeEntry(action).title }}</span>
          <span class="subtitle">{{ describeEntry(action).subtitle }}</span>
        </span>
      </button>
      <p v-if="shownOtherLogin.length > 0" class="section" data-testid="inline-other-title">Other logins in Hisar</p>
      <button
        v-for="(other, index) in shownOtherLogin"
        :key="`${other.vaultID}/${other.itemID}`"
        type="button"
        class="item"
        :class="{ selected: selectedIndex === shownEntry.length + index }"
        data-testid="inline-other"
        @click="chooseAction({ kind: 'other', login: other }, $event)"
      >
        <span class="avatar other" aria-hidden="true">{{ (other.title || "?").slice(0, 1).toUpperCase() }}</span>
        <span class="text">
          <span class="title">{{ other.title }}</span>
          <span class="subtitle">{{ other.username || "No username" }}{{ other.host ? ` · saved for ${other.host}` : "" }}</span>
        </span>
      </button>
      <button
        v-if="isExpandOffered"
        type="button"
        class="item more"
        :class="{ selected: selectedIndex === shownEntry.length }"
        data-testid="inline-show-all"
        @click="chooseAction({ kind: 'expand' }, $event)"
      >
        <template v-if="hiddenCount > 0">
          <span>Show all {{ allEntry.length }} {{ entryNoun }}</span>
          <span class="more-count">+{{ hiddenCount }}</span>
        </template>
        <span v-else>{{ allEntry.length === 0 ? "Find a login in Hisar…" : "Find another login in Hisar…" }}</span>
      </button>
      <p
        v-if="isExpanded && filterText.trim() !== '' && shownEntry.length === 0 && shownOtherLogin.length === 0"
        class="message muted"
        data-testid="inline-no-matches"
      >
        Nothing matches "{{ filterText.trim() }}".
      </p>
      <template v-if="allEntry.length === 0 && !isExpanded">
        <div v-if="menuState.purpose === 'identity'" class="message" data-testid="inline-no-identities">
          <p class="title">No identities in Hisar</p>
          <p class="muted">Add an identity in Hisar to fill your name, email and address.</p>
        </div>
        <div v-else-if="menuState.purpose === 'card'" class="message" data-testid="inline-no-cards">
          <p class="title">No cards in Hisar</p>
          <p class="muted">Add a credit card in Hisar to fill payment forms.</p>
        </div>
        <div v-else-if="menuState.purpose === 'address'" class="message" data-testid="inline-no-addresses">
          <p class="title">No addresses in Hisar</p>
          <p class="muted">Add an address, or one to an identity, in Hisar.</p>
        </div>
        <div v-else-if="isOneTimeCodeMenu" class="message" data-testid="inline-no-codes">
          <p class="title">No one-time codes for {{ menuState.host }}</p>
          <p class="muted">Add the setup key to the login in Hisar.</p>
        </div>
        <div v-else class="message" data-testid="inline-no-logins">
          <p class="title">No logins for {{ menuState.host }}</p>
          <p class="muted">Hisar offers to save the login after you sign in.</p>
        </div>
      </template>
    </template>
    <template v-else-if="menuState">
      <div class="message">
        <p class="title" data-testid="inline-status">
          {{
            menuState.view === "update-required"
              ? menuState.updateRequired?.target === "extension"
                ? "Update the Hisar extension"
                : "Hisar needs an update"
              : menuState.view === "not-paired"
              ? "This browser is not paired with Hisar"
              : menuState.view === "locked"
                ? "Hisar is locked"
                : menuState.view === "signed-out"
                  ? "Sign in to Hisar on this computer"
                  : "Hisar is not running"
          }}
        </p>
      </div>
      <button
        v-if="allAction.length > 0"
        type="button"
        class="item action"
        :class="{ selected: selectedIndex === 0 }"
        :disabled="isWaiting"
        data-testid="inline-action"
        @click="chooseAction(allAction[0], $event)"
      >
        {{
          menuState.view === "update-required"
            ? "Show details…"
            : menuState.view === "not-paired"
            ? "Pair with Hisar"
            : menuState.view === "locked"
              ? isWaiting
                ? "Waiting for Hisar…"
                : "Unlock Hisar"
              : isWaiting
                ? "Opening Hisar…"
                : "Open Hisar"
        }}
      </button>
    </template>
  </div>
</template>

<style>
html,
body {
  margin: 0;
  padding: 0;
  background: transparent;
  overflow: hidden;
}

body {
  font: 13px/1.45 -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  color: #e8eaed;
  color-scheme: dark;
  -webkit-font-smoothing: antialiased;
  user-select: none;
}

/* A menu cut to the room beside the field scrolls inside, so it stays wholly in its frame. */
.menu {
  display: flex;
  flex-direction: column;
  gap: 2px;
  box-sizing: border-box;
  max-height: 100vh;
  overflow-y: auto;
  padding: 6px;
  border: 1px solid #30343a;
  border-radius: 10px;
  background: #111214;
}

.menu.busy .item {
  opacity: 0.6;
  cursor: progress;
}

.item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 7px 8px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.item:hover:not(:disabled),
.item.selected {
  background: #262a30;
}

.item:focus-visible {
  outline: 2px solid #8a95a5;
  outline-offset: -2px;
}

.item.more {
  justify-content: space-between;
  color: #b4b9c0;
}

.section {
  margin: 6px 8px 2px;
  color: #7c838d;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.avatar.other {
  border-style: dashed;
}

.confirm {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px 4px 2px;
}

.confirm p {
  margin: 0 4px;
}

.confirm .title {
  white-space: normal;
}

.confirm .muted {
  margin-bottom: 4px;
}

.item.cancel {
  justify-content: center;
  color: #b4b9c0;
}

.more-count {
  padding: 1px 7px;
  border: 1px solid #30343a;
  border-radius: 10px;
  color: #7c838d;
  font-size: 11px;
}

/* The filter stays at the top while the expanded list scrolls under it. */
.filter {
  position: sticky;
  top: -6px;
  z-index: 1;
  margin: -6px -6px 2px;
  padding: 6px 6px 4px;
  background: #111214;
}

.filter-icon {
  position: absolute;
  top: 50%;
  left: 15px;
  transform: translateY(-40%);
  color: #7c838d;
}

.filter input {
  box-sizing: border-box;
  width: 100%;
  padding: 6px 8px 6px 28px;
  border: 1px solid #30343a;
  border-radius: 7px;
  background: #1c1f23;
  color: inherit;
  font: inherit;
  outline: none;
  user-select: text;
}

.filter input:focus {
  border-color: #8a95a5;
}

.item.action {
  justify-content: center;
  border: 1px solid #eceef1;
  background: #eceef1;
  color: #0b0c0e;
  font-weight: 600;
}

.item.action:hover:not(:disabled),
.item.action.selected {
  background: #ffffff;
}

.item:disabled {
  opacity: 0.6;
  cursor: default;
}

.avatar {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 1px solid #30343a;
  border-radius: 7px;
  background: #1c1f23;
  color: #b4b9c0;
  font-size: 12px;
  font-weight: 600;
}

.text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.title,
.subtitle {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title {
  margin: 0;
  font-weight: 500;
}

.subtitle {
  font-size: 12px;
  color: #7c838d;
}

.monospace {
  font-family: ui-monospace, "SF Mono", SFMono-Regular, Menlo, monospace;
}

.message {
  margin: 0;
  padding: 6px 8px;
}

.message p {
  margin: 0;
}

.muted {
  color: #7c838d;
  font-size: 12px;
}

.error {
  color: #f2a7a9;
  font-size: 12px;
}
</style>
