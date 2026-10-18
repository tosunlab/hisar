<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import type { ItemSummary, NewItem, VaultSummary } from "../../shared/protocol";
import { describeError, sendToBackground } from "../api";
import AppIcon from "./AppIcon.vue";

// pageUrl is the tab's address: a new login starts with its site as the title and website, and a generated password.
const props = defineProps<{ pageUrl: string }>();
const emit = defineEmits<{ cancel: []; created: [item: ItemSummary] }>();

type Category = NewItem["category"];

const allCategory: { id: Category; label: string }[] = [
  { id: "login", label: "Login" },
  { id: "secure-note", label: "Note" },
  { id: "credit-card", label: "Card" },
  { id: "identity", label: "Identity" },
];

const pageOrigin = computed(() => {
  try {
    const url = new URL(props.pageUrl);
    return url.protocol === "https:" || url.protocol === "http:" ? url.origin : "";
  } catch {
    return "";
  }
});

const category = ref<Category>("login");
const allVault = ref<VaultSummary[]>([]);
const vaultID = ref("");
const title = ref(pageOrigin.value ? new URL(pageOrigin.value).hostname.replace(/^www\./, "") : "");
const username = ref("");
const password = ref("");
const isPasswordShown = ref(false);
const url = ref(pageOrigin.value);
const notes = ref("");
const cardholder = ref("");
const cardNumber = ref("");
const expiry = ref("");
const securityCode = ref("");
const firstName = ref("");
const lastName = ref("");
const email = ref("");
const phone = ref("");
const errorMessage = ref("");
const isSaving = ref(false);
const titleInput = ref<HTMLInputElement>();

async function generatePassword(): Promise<void> {
  try {
    password.value = await sendToBackground<string>({ type: "popup.generatePassword", options: {} });
  } catch (error) {
    errorMessage.value = describeError(error);
  }
}

function buildItem(): NewItem {
  const item: NewItem = { category: category.value, title: title.value.trim(), notes: notes.value || undefined };
  switch (category.value) {
    case "login":
      return { ...item, username: username.value.trim(), password: password.value, url: url.value.trim() };
    case "credit-card":
      return { ...item, cardholder: cardholder.value.trim(), cardNumber: cardNumber.value.trim(), expiry: expiry.value.trim(), securityCode: securityCode.value.trim() };
    case "identity":
      return { ...item, firstName: firstName.value.trim(), lastName: lastName.value.trim(), email: email.value.trim(), phone: phone.value.trim() };
    default:
      return item;
  }
}

async function save(): Promise<void> {
  if (!title.value.trim() || !vaultID.value) return;
  isSaving.value = true;
  errorMessage.value = "";
  try {
    const created = await sendToBackground<ItemSummary>({ type: "popup.createItem", vaultID: vaultID.value, item: buildItem() });
    emit("created", created);
  } catch (error) {
    errorMessage.value = describeError(error);
  } finally {
    isSaving.value = false;
  }
}

onMounted(async () => {
  titleInput.value?.focus();
  try {
    allVault.value = await sendToBackground<VaultSummary[]>({ type: "popup.listVaults" });
    vaultID.value = allVault.value[0]?.vaultID ?? "";
  } catch (error) {
    errorMessage.value = describeError(error);
  }
  await generatePassword();
});
</script>

<template>
  <form class="new-item" data-testid="new-item" @submit.prevent="save">
    <div class="top">
      <button type="button" class="ghost icon-button" title="Back" aria-label="Back" data-testid="new-item-cancel" @click="emit('cancel')">
        <AppIcon name="back" />
      </button>
      <p class="heading">New item</p>
    </div>
    <div class="chips" role="radiogroup" aria-label="Kind of item">
      <button
        v-for="choice in allCategory"
        :key="choice.id"
        type="button"
        role="radio"
        class="chip"
        :class="{ active: category === choice.id }"
        :aria-checked="category === choice.id"
        :data-testid="`new-${choice.id}`"
        @click="category = choice.id"
      >
        {{ choice.label }}
      </button>
    </div>
    <label class="field">
      <span>Title</span>
      <input ref="titleInput" v-model="title" type="text" required data-testid="new-title" />
    </label>
    <template v-if="category === 'login'">
      <label class="field">
        <span>Username</span>
        <input v-model="username" type="text" autocomplete="off" data-testid="new-username" />
      </label>
      <label class="field">
        <span>Password</span>
        <span class="with-buttons">
          <input v-model="password" :type="isPasswordShown ? 'text' : 'password'" class="monospace" autocomplete="off" data-testid="new-password" />
          <button type="button" class="ghost icon-button" :aria-label="isPasswordShown ? 'Hide password' : 'Show password'" @click="isPasswordShown = !isPasswordShown">
            <AppIcon :name="isPasswordShown ? 'eye-off' : 'eye'" />
          </button>
          <button type="button" class="ghost icon-button" aria-label="Generate another password" title="Generate another password" data-testid="new-generate" @click="generatePassword">
            <AppIcon name="refresh" />
          </button>
        </span>
      </label>
      <label class="field">
        <span>Website</span>
        <input v-model="url" type="text" placeholder="https://" data-testid="new-url" />
      </label>
    </template>
    <template v-else-if="category === 'credit-card'">
      <label class="field">
        <span>Name on card</span>
        <input v-model="cardholder" type="text" autocomplete="off" data-testid="new-cardholder" />
      </label>
      <label class="field">
        <span>Card number</span>
        <input v-model="cardNumber" type="text" inputmode="numeric" autocomplete="off" class="monospace" data-testid="new-card-number" />
      </label>
      <div class="row">
        <label class="field">
          <span>Expiry</span>
          <input v-model="expiry" type="text" placeholder="MM/YY" autocomplete="off" data-testid="new-expiry" />
        </label>
        <label class="field">
          <span>Security code</span>
          <input v-model="securityCode" type="text" inputmode="numeric" autocomplete="off" data-testid="new-security-code" />
        </label>
      </div>
    </template>
    <template v-else-if="category === 'identity'">
      <div class="row">
        <label class="field">
          <span>First name</span>
          <input v-model="firstName" type="text" autocomplete="off" data-testid="new-first-name" />
        </label>
        <label class="field">
          <span>Last name</span>
          <input v-model="lastName" type="text" autocomplete="off" data-testid="new-last-name" />
        </label>
      </div>
      <label class="field">
        <span>Email</span>
        <input v-model="email" type="text" autocomplete="off" data-testid="new-email" />
      </label>
      <label class="field">
        <span>Phone</span>
        <input v-model="phone" type="text" autocomplete="off" data-testid="new-phone" />
      </label>
    </template>
    <label class="field">
      <span>Notes</span>
      <textarea v-model="notes" rows="3" data-testid="new-notes"></textarea>
    </label>
    <label v-if="allVault.length > 1" class="field">
      <span>Vault</span>
      <select v-model="vaultID" data-testid="new-vault">
        <option v-for="vault in allVault" :key="vault.vaultID" :value="vault.vaultID">{{ vault.name }}</option>
      </select>
    </label>
    <p v-if="errorMessage" class="error" data-testid="new-item-error">{{ errorMessage }}</p>
    <button type="submit" class="primary save" :disabled="isSaving || !title.trim() || !vaultID" data-testid="new-item-save">
      {{ isSaving ? "Saving…" : "Save" }}
    </button>
  </form>
</template>

<style scoped>
.new-item {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.heading {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}

.chips {
  display: flex;
  gap: 4px;
}

.chip {
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

.field {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.field > span:first-child {
  color: var(--color-text-muted);
  font-size: 11px;
}

.row {
  display: flex;
  gap: 8px;
}

.with-buttons {
  display: flex;
  align-items: center;
  gap: 2px;
}

.field input:not([type="search"]),
.field select,
.field textarea {
  width: 100%;
  min-width: 0;
  padding: 7px 9px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  color: inherit;
  font: inherit;
}

.field textarea {
  resize: vertical;
}

.field input:focus,
.field select:focus,
.field textarea:focus {
  outline: none;
  border-color: var(--color-focus);
}

.error {
  margin: 0;
}

.save {
  width: 100%;
}
</style>
