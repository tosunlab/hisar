<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { VisibilityGuard } from "../shared/click-guard";
import type { InlineMenuBroadcast, InlineMenuState } from "../shared/messages";
import type { AddressSummary, CardSummary, IdentitySummary, LoginSummary } from "../shared/protocol";
import { sendToBackground } from "./api";

const props = withDefaults(
  defineProps<{
    sessionID: string;
    isVisibilityTracked?: boolean;
    pollIntervalMilliseconds?: number;
    pollLimit?: number;
    now?: () => number;
    isEventTrusted?: (event: Event) => boolean;
  }>(),
  {
    isVisibilityTracked: false,
    pollIntervalMilliseconds: 1000,
    pollLimit: 120,
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
  | { kind: "suggest" }
  | { kind: "pair" }
  | { kind: "unlock" }
  | { kind: "open-hisar" };

const menuState = ref<InlineMenuState>();
const selectedIndex = ref(-1);
const errorMessage = ref("");
const isWaiting = ref(false);
const root = ref<HTMLElement>();
const guard = new VisibilityGuard();
let observer: IntersectionObserver | undefined;
let pollTimer: ReturnType<typeof setTimeout> | undefined;
let lastReportedHeight = 0;

const isOneTimeCodeMenu = computed(() => menuState.value?.purpose === "one-time-code");
const allAddress = computed(() => menuState.value?.allAddress ?? []);
const allCard = computed(() => menuState.value?.allCard ?? []);
const allIdentity = computed(() => menuState.value?.allIdentity ?? []);

function describeCard(card: CardSummary): string {
  return [card.brand, card.last4 ? `•••• ${card.last4}` : "", card.expiry ? `expires ${card.expiry}` : ""].filter(Boolean).join(" · ") || "Card";
}

const allAction = computed<MenuAction[]>(() => {
  const state = menuState.value;
  if (!state) return [];
  switch (state.view) {
    case "logins":
      if (state.purpose === "new-password") return state.suggestedPassword ? [{ kind: "suggest" }] : [];
      if (state.purpose === "identity") {
        return [
          ...(state.allIdentity ?? []).map((identity): MenuAction => ({ kind: "identity", identity })),
          ...(state.hasMoreIdentity ? [{ kind: "more" } as MenuAction] : []),
        ];
      }
      if (state.purpose === "card") {
        return [...(state.allCard ?? []).map((card): MenuAction => ({ kind: "card", card })), ...(state.hasMoreCard ? [{ kind: "more" } as MenuAction] : [])];
      }
      if (state.purpose === "address") {
        return [
          ...(state.allAddress ?? []).map((address): MenuAction => ({ kind: "address", address })),
          ...(state.hasMoreAddress ? [{ kind: "more" } as MenuAction] : []),
        ];
      }
      return [
        ...state.allLogin.map((login): MenuAction => ({ kind: state.purpose === "one-time-code" ? "code" : "login", login })),
        ...(state.hasMoreLogin ? [{ kind: "more" } as MenuAction] : []),
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

async function runAction(action: MenuAction): Promise<void> {
  errorMessage.value = "";
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
  if (event.key === "Escape") void sendToBackground({ type: "inlineMenu.close", sessionID: props.sessionID }).catch(() => undefined);
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

watch([menuState, errorMessage], reportHeight);
watch(allAction, () => (selectedIndex.value = -1));
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
  observer?.disconnect();
  chrome.runtime.onMessage.removeListener(handleBroadcast);
  window.removeEventListener("keydown", handleKeydown);
});

defineExpose({ pressKey });
</script>

<template>
  <div ref="root" class="menu" role="listbox" aria-label="Hisar">
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
    <template v-else-if="menuState?.view === 'logins' && menuState.purpose === 'identity'">
      <button
        v-for="(identity, index) in allIdentity"
        :key="identity.vaultID + identity.itemID"
        type="button"
        class="item"
        :class="{ selected: selectedIndex === index }"
        data-testid="inline-identity"
        @click="chooseAction({ kind: 'identity', identity }, $event)"
      >
        <span class="avatar key" aria-hidden="true">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 5h18v14H3zM9 8.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM5.5 16.5c.6-1.6 1.9-2.3 3.5-2.3s2.9.7 3.5 2.3M15 10h4M15 14h4" /></svg>
        </span>
        <span class="text">
          <span class="title">{{ identity.title }}</span>
          <span class="subtitle">{{ identity.summary || "Identity" }}</span>
        </span>
      </button>
      <button
        v-if="menuState.hasMoreIdentity"
        type="button"
        class="item more"
        :class="{ selected: selectedIndex === allIdentity.length }"
        data-testid="inline-more"
        @click="chooseAction({ kind: 'more' }, $event)"
      >
        More in Hisar…
      </button>
      <div v-if="allIdentity.length === 0" class="message" data-testid="inline-no-identities">
        <p class="title">No identities in Hisar</p>
        <p class="muted">Add an identity in Hisar to fill your name, email and address.</p>
      </div>
    </template>
    <template v-else-if="menuState?.view === 'logins' && menuState.purpose === 'card'">
      <button
        v-for="(card, index) in allCard"
        :key="card.vaultID + card.itemID"
        type="button"
        class="item"
        :class="{ selected: selectedIndex === index }"
        data-testid="inline-card"
        @click="chooseAction({ kind: 'card', card }, $event)"
      >
        <span class="avatar key" aria-hidden="true">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18v12H3zM3 10h18M7 15h4" /></svg>
        </span>
        <span class="text">
          <span class="title">{{ card.title }}</span>
          <span class="subtitle">{{ describeCard(card) }}</span>
        </span>
      </button>
      <button
        v-if="menuState.hasMoreCard"
        type="button"
        class="item more"
        :class="{ selected: selectedIndex === allCard.length }"
        data-testid="inline-more"
        @click="chooseAction({ kind: 'more' }, $event)"
      >
        More in Hisar…
      </button>
      <div v-if="allCard.length === 0" class="message" data-testid="inline-no-cards">
        <p class="title">No cards in Hisar</p>
        <p class="muted">Add a credit card in Hisar to fill payment forms.</p>
      </div>
    </template>
    <template v-else-if="menuState?.view === 'logins' && menuState.purpose === 'address'">
      <button
        v-for="(address, index) in allAddress"
        :key="address.vaultID + address.itemID"
        type="button"
        class="item"
        :class="{ selected: selectedIndex === index }"
        data-testid="inline-address"
        @click="chooseAction({ kind: 'address', address }, $event)"
      >
        <span class="avatar key" aria-hidden="true">
          <svg v-if="address.source === 'identity'" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 5h18v14H3zM9 8.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM5.5 16.5c.6-1.6 1.9-2.3 3.5-2.3s2.9.7 3.5 2.3M15 10h4M15 14h4" /></svg>
          <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 21s-7-6.1-7-11.5a7 7 0 0 1 14 0C19 14.9 12 21 12 21zM12 7.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4z" /></svg>
        </span>
        <span class="text">
          <span class="title">{{ address.title }}</span>
          <span class="subtitle">{{ [address.summary, address.source === "identity" ? "Identity" : ""].filter(Boolean).join(" · ") || "Address" }}</span>
        </span>
      </button>
      <button
        v-if="menuState.hasMoreAddress"
        type="button"
        class="item more"
        :class="{ selected: selectedIndex === allAddress.length }"
        data-testid="inline-more"
        @click="chooseAction({ kind: 'more' }, $event)"
      >
        More in Hisar…
      </button>
      <div v-if="allAddress.length === 0" class="message" data-testid="inline-no-addresses">
        <p class="title">No addresses in Hisar</p>
        <p class="muted">Add an address, or one to an identity, in Hisar.</p>
      </div>
    </template>
    <template v-else-if="menuState?.view === 'logins'">
      <button
        v-for="(login, index) in menuState.allLogin"
        :key="login.vaultID + login.itemID"
        type="button"
        class="item"
        :class="{ selected: selectedIndex === index }"
        :data-testid="isOneTimeCodeMenu ? 'inline-code' : 'inline-login'"
        @click="chooseAction({ kind: isOneTimeCodeMenu ? 'code' : 'login', login }, $event)"
      >
        <span v-if="isOneTimeCodeMenu" class="avatar key" aria-hidden="true">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="13" r="8" /><path d="M12 9v4l2.5 2.5M10 2h4" /></svg>
        </span>
        <span v-else class="avatar" aria-hidden="true">{{ (login.title || "?").slice(0, 1).toUpperCase() }}</span>
        <span class="text">
          <span class="title">{{ login.title }}</span>
          <span class="subtitle">{{ isOneTimeCodeMenu ? `One-time code · ${login.username || "no username"}` : login.username || "No username" }}</span>
        </span>
      </button>
      <button
        v-if="menuState.hasMoreLogin"
        type="button"
        class="item more"
        :class="{ selected: selectedIndex === menuState.allLogin.length }"
        data-testid="inline-more"
        @click="chooseAction({ kind: 'more' }, $event)"
      >
        More in Hisar…
      </button>
      <div v-if="menuState.allLogin.length === 0 && isOneTimeCodeMenu" class="message" data-testid="inline-no-codes">
        <p class="title">No one-time codes for {{ menuState.host }}</p>
        <p class="muted">Add the setup key to the login in Hisar.</p>
      </div>
      <div v-else-if="menuState.allLogin.length === 0" class="message" data-testid="inline-no-logins">
        <p class="title">No logins for {{ menuState.host }}</p>
        <p class="muted">Hisar offers to save the login after you sign in.</p>
      </div>
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
    <p v-if="errorMessage" class="message error" data-testid="inline-error">{{ errorMessage }}</p>
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
  color: #b4b9c0;
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
