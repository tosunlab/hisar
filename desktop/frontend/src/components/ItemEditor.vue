<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from "vue";
import { api, createEmptyCard, createEmptyIdentity, createEmptyItem, type CardDescription, type ItemDetail } from "../api";
import { allCardBrand, creditCardCategory, identityCategory, formatCardNumberInput, formatExpiryInput, formatInputAsTyped, getItemIcon, isCalendarDate, secureNoteCategory, sshKeyCategory } from "../util";
import { cancelEditing, saveItem, store } from "../store";
import AppIcon from "./AppIcon.vue";
import ItemTile from "./ItemTile.vue";
import PasswordGenerator from "./PasswordGenerator.vue";
import StrengthMeter from "./StrengthMeter.vue";

function createDraft(): ItemDetail {
  if (store.editorMode === "edit" && store.selectedItem) {
    const selected = store.selectedItem;
    return {
      ...selected,
      urls: [...selected.urls],
      fields: selected.fields.map((field) => ({ ...field })),
      passkeys: [...(selected.passkeys ?? [])],
      card: selected.card ? { ...selected.card } : selected.category === creditCardCategory ? createEmptyCard() : selected.card,
      identity: selected.identity ? { ...selected.identity } : selected.category === identityCategory ? createEmptyIdentity() : selected.identity,
    };
  }
  const empty = createEmptyItem();
  if (store.editorMode === "create-secure-note") {
    empty.category = secureNoteCategory;
  } else if (store.editorMode === "create-credit-card") {
    empty.category = creditCardCategory;
    empty.card = createEmptyCard();
  } else if (store.editorMode === "create-identity") {
    empty.category = identityCategory;
    empty.identity = createEmptyIdentity();
  }
  return empty;
}

const draft = reactive<ItemDetail>(createDraft());
const urlText = ref(draft.urls.join("\n"));
const isPasswordVisible = ref(false);
const isGeneratorOpen = ref(store.editorMode === "create");
const isSSHKey = draft.category === sshKeyCategory;
const isSecureNote = draft.category === secureNoteCategory;
const isCreditCard = draft.category === creditCardCategory;
const isIdentity = draft.category === identityCategory;
const isDocumentNumberVisible = ref(false);
// A birth date imported in another form, such as 04/12/1990, or with no such day, such as 2023-02-31, stays editable as
// text; a date picker would show it empty.
const isBirthDateText = Boolean(draft.identity?.birthDate) && !isCalendarDate(draft.identity?.birthDate ?? "");
const isCreating = store.editorMode.startsWith("create");
const isCardSecretVisible = ref(false);
const cardDescription = ref<CardDescription>({
  brand: draft.card?.brand ?? "",
  last4: draft.card?.last4 ?? "",
  numberWarning: draft.card?.numberWarning ?? "",
  expiryWarning: draft.card?.expiryWarning ?? "",
});

// The types the picker offers: the known brands, plus a type saved from elsewhere that is not one of them.
const allCardType = computed(() => {
  const cardType = draft.card?.type ?? "";
  return cardType === "" || allCardBrand.includes(cardType) ? allCardBrand : [...allCardBrand, cardType];
});
// A type the user chose that the number does not match; the card still saves.
const cardTypeMismatch = computed(() => {
  const cardType = draft.card?.type ?? "";
  const brand = cardDescription.value.brand;
  return cardType !== "" && brand !== "" && cardType !== brand ? `The number looks like a ${brand} card` : "";
});

// The backend checks the number and expiry, so the brand and warnings follow what Hisar saves. The type follows the
// number's brand until the user picks a different one.
let numberBrand = cardDescription.value.brand;
let describeTimer: ReturnType<typeof setTimeout> | undefined;
watch(
  () => [draft.card?.number, draft.card?.expiry],
  () => {
    clearTimeout(describeTimer);
    describeTimer = setTimeout(async () => {
      try {
        cardDescription.value = await api.describeCard(draft.card?.number ?? "", draft.card?.expiry ?? "");
        if (draft.card && (draft.card.type === "" || draft.card.type === numberBrand)) {
          draft.card.type = cardDescription.value.brand;
        }
        numberBrand = cardDescription.value.brand;
      } catch {
        // A failed check only hides the hint; saving still works.
      }
    }, 250);
  },
);
onBeforeUnmount(() => clearTimeout(describeTimer));

// The number and expiry are formatted as they are typed, the way Hisar saves them.
function formatCardInput(event: Event, key: "number" | "expiry", format: (value: string) => string): void {
  if (draft.card) {
    draft.card[key] = formatInputAsTyped(event.target as HTMLInputElement, format, draft.card[key], (event as InputEvent).inputType ?? "");
  }
}

function usePassword(password: string): void {
  draft.password = password;
  isGeneratorOpen.value = false;
  isPasswordVisible.value = true;
}

function addField(): void {
  draft.fields.push({ name: "", value: "", concealed: false });
}

function removeField(index: number): void {
  draft.fields.splice(index, 1);
}

async function submitItem(): Promise<void> {
  draft.urls = urlText.value
    .split("\n")
    .map((link) => link.trim())
    .filter((link) => link !== "");
  await saveItem({ ...draft });
}
</script>

<template>
  <form class="item-editor" @submit.prevent="submitItem">
    <header>
      <ItemTile :title="draft.title || '?'" :size="52" :icon="getItemIcon(draft.category)" />
      <input v-model="draft.title" class="title-input" name="title" :placeholder="isSecureNote ? 'Note title' : 'Item title'" aria-label="Title" required autofocus />
    </header>

    <section v-if="isSecureNote" class="card editor-card" data-testid="secure-note-editor">
      <label class="last">
        <span>Notes</span>
        <textarea v-model="draft.notes" name="notes" rows="12" placeholder="Anything worth keeping safe: recovery codes, door codes, account details"></textarea>
      </label>
    </section>

    <section v-if="isCreditCard && draft.card" class="card editor-card" data-testid="credit-card-editor">
      <label>
        <span>Cardholder</span>
        <input v-model="draft.card.cardholder" name="cardholder" autocomplete="off" placeholder="Name on the card" />
      </label>
      <label>
        <span>Card number</span>
        <input
          :value="draft.card.number"
          name="cardNumber"
          class="monospace"
          :type="isCardSecretVisible ? 'text' : 'password'"
          inputmode="numeric"
          autocomplete="off"
          placeholder="1234 5678 9012 3456"
          @input="formatCardInput($event, 'number', formatCardNumberInput)"
        />
        <small v-if="cardDescription.numberWarning" class="hint warning-text" data-testid="card-number-warning">{{ cardDescription.numberWarning }}</small>
      </label>
      <label>
        <span>Type</span>
        <select v-model="draft.card.type" name="cardType">
          <option value="">Not set</option>
          <option v-for="cardType in allCardType" :key="cardType" :value="cardType">{{ cardType }}</option>
        </select>
        <small v-if="cardTypeMismatch" class="hint warning-text" data-testid="card-type-warning">{{ cardTypeMismatch }}</small>
      </label>
      <div class="row card-row">
        <label>
          <span>Expiry</span>
          <input :value="draft.card.expiry" name="expiry" inputmode="numeric" autocomplete="off" placeholder="MM/YY" @input="formatCardInput($event, 'expiry', formatExpiryInput)" />
          <small v-if="cardDescription.expiryWarning" class="hint warning-text" data-testid="card-expiry-warning">{{ cardDescription.expiryWarning }}</small>
        </label>
        <label>
          <span>Security code</span>
          <input v-model="draft.card.securityCode" name="securityCode" class="monospace" :type="isCardSecretVisible ? 'text' : 'password'" inputmode="numeric" autocomplete="off" />
        </label>
        <label>
          <span>PIN</span>
          <input v-model="draft.card.pin" name="pin" class="monospace" :type="isCardSecretVisible ? 'text' : 'password'" inputmode="numeric" autocomplete="off" />
        </label>
      </div>
      <button type="button" class="ghost reveal-card" @click="isCardSecretVisible = !isCardSecretVisible">
        <AppIcon :name="isCardSecretVisible ? 'eyeOff' : 'eye'" /> {{ isCardSecretVisible ? "Hide number and codes" : "Show number and codes" }}
      </button>
      <label class="last">
        <span>Billing address</span>
        <textarea v-model="draft.card.billingAddress" name="billingAddress" rows="2"></textarea>
      </label>
    </section>

    <section v-if="isIdentity && draft.identity" class="card editor-card" data-testid="identity-editor">
      <div class="row card-row">
        <label>
          <span>First name</span>
          <input v-model="draft.identity.firstName" name="firstName" autocomplete="off" />
        </label>
        <label>
          <span>Middle name</span>
          <input v-model="draft.identity.middleName" name="middleName" autocomplete="off" />
        </label>
        <label>
          <span>Last name</span>
          <input v-model="draft.identity.lastName" name="lastName" autocomplete="off" />
        </label>
      </div>
      <div class="row card-row">
        <label>
          <span>Birth date</span>
          <input v-model="draft.identity.birthDate" name="birthDate" :type="isBirthDateText ? 'text' : 'date'" autocomplete="off" />
        </label>
        <label>
          <span>Email</span>
          <!-- Not type="email": the browser would refuse to save a value it does not take for one address, such as one saved
               on the iPhone or imported. -->
          <input v-model="draft.identity.email" name="email" inputmode="email" autocomplete="off" spellcheck="false" />
        </label>
        <label>
          <span>Phone</span>
          <input v-model="draft.identity.phone" name="phone" type="tel" autocomplete="off" />
        </label>
      </div>
      <label>
        <span>Address</span>
        <textarea v-model="draft.identity.address" name="address" rows="2"></textarea>
      </label>
      <div class="row card-row">
        <label>
          <span>Passport number</span>
          <input v-model="draft.identity.passportNumber" name="passportNumber" class="monospace" :type="isDocumentNumberVisible ? 'text' : 'password'" autocomplete="off" />
        </label>
        <label>
          <span>Driver's license number</span>
          <input v-model="draft.identity.driversLicenseNumber" name="driversLicenseNumber" class="monospace" :type="isDocumentNumberVisible ? 'text' : 'password'" autocomplete="off" />
        </label>
        <label>
          <span>National ID number</span>
          <input v-model="draft.identity.nationalIdNumber" name="nationalIdNumber" class="monospace" :type="isDocumentNumberVisible ? 'text' : 'password'" autocomplete="off" />
        </label>
      </div>
      <button type="button" class="ghost reveal-card last" @click="isDocumentNumberVisible = !isDocumentNumberVisible">
        <AppIcon :name="isDocumentNumberVisible ? 'eyeOff' : 'eye'" /> {{ isDocumentNumberVisible ? "Hide document numbers" : "Show document numbers" }}
      </button>
    </section>

    <p v-if="isSSHKey" class="card ssh-key-note muted" data-testid="ssh-key-note">
      <AppIcon name="key" :size="14" /> The key itself cannot be changed. To replace it, add a new SSH key.
    </p>

    <section v-if="!isSSHKey && !isSecureNote && !isCreditCard && !isIdentity" class="card editor-card">
      <label>
        <span>Username</span>
        <input v-model="draft.username" name="username" autocomplete="off" placeholder="Username or email" />
      </label>
      <label>
        <span>Password</span>
        <span class="row">
          <input
            v-model="draft.password"
            name="password"
            class="monospace"
            :type="isPasswordVisible ? 'text' : 'password'"
            autocomplete="new-password"
          />
          <button
            type="button"
            class="icon-button"
            :aria-label="isPasswordVisible ? 'Hide password' : 'Show password'"
            @click="isPasswordVisible = !isPasswordVisible"
          >
            <AppIcon :name="isPasswordVisible ? 'eyeOff' : 'eye'" />
          </button>
          <button type="button" :class="{ active: isGeneratorOpen }" @click="isGeneratorOpen = !isGeneratorOpen">
            <AppIcon name="refresh" /> Generate
          </button>
        </span>
        <StrengthMeter :password="draft.password" />
      </label>
      <PasswordGenerator v-if="isGeneratorOpen" @use="usePassword" @close="isGeneratorOpen = false" />
      <label>
        <span>Websites</span>
        <textarea v-model="urlText" name="urls" rows="2" placeholder="https://example.com (one per line)"></textarea>
      </label>
      <label class="last">
        <span>One-time password</span>
        <input
          v-model="draft.oneTimePassword"
          name="oneTimePassword"
          class="monospace"
          autocomplete="off"
          spellcheck="false"
          placeholder="Setup key or otpauth:// link from the site's two-factor setup"
        />
      </label>
    </section>

    <section v-if="!isSSHKey" class="card editor-card">
      <p class="section-label">Other fields</p>
      <div v-for="(field, index) in draft.fields" :key="index" class="custom-field">
        <input v-model="field.name" :name="`fieldName${index}`" placeholder="Name" aria-label="Field name" />
        <input
          v-model="field.value"
          :name="`fieldValue${index}`"
          :type="field.concealed ? 'password' : 'text'"
          placeholder="Value"
          aria-label="Field value"
        />
        <label class="concealed" :title="'Hide this value until revealed'">
          <input v-model="field.concealed" type="checkbox" /> <span>Secret</span>
        </label>
        <button type="button" class="ghost icon-button" aria-label="Remove field" @click="removeField(index)"><AppIcon name="close" /></button>
      </div>
      <button type="button" class="ghost add-field" @click="addField"><AppIcon name="plus" /> Add field</button>
    </section>

    <section v-if="(draft.passkeys ?? []).length > 0" class="card editor-card" data-testid="editor-passkeys">
      <p class="section-label">Passkeys</p>
      <p v-for="passkey in draft.passkeys" :key="passkey.credentialId" class="passkey-line">
        <AppIcon name="passkey" :size="14" /> {{ passkey.relyingPartyId }} <span class="muted">{{ passkey.userName }}</span>
      </p>
      <p class="muted passkey-note">Passkeys stay as they are when you save. Delete one from the item view.</p>
    </section>

    <section v-if="!isSecureNote" class="card editor-card">
      <label class="last">
        <span>Notes</span>
        <textarea v-model="draft.notes" name="notes" rows="4" placeholder="Anything else worth keeping safe"></textarea>
      </label>
    </section>

    <div class="row footer">
      <span class="muted"><AppIcon name="shield" :size="12" /> Encrypted on this device before saving</span>
      <div class="spacer"></div>
      <button type="button" class="ghost" @click="cancelEditing">Cancel</button>
      <button class="primary" type="submit" :disabled="store.isBusy">{{ isCreating ? "Add item" : "Save" }}</button>
    </div>
  </form>
</template>

<style scoped>
.item-editor {
  max-width: 640px;
}

header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 18px;
}

.title-input {
  padding: 6px 8px;
  border-color: transparent;
  background: transparent;
  font-size: 20px;
  font-weight: 650;
}

.title-input:hover {
  border-color: var(--color-border);
}

.editor-card {
  margin-bottom: 12px;
  padding: 16px;
}

label.last {
  margin-bottom: 0;
}

.hint {
  display: block;
  margin-top: 4px;
  font-size: 12px;
}

.warning-text {
  color: var(--color-warning);
}

.card-row {
  align-items: flex-start;
  gap: 12px;
}

.card-row label {
  flex: 1;
}

.reveal-card {
  margin: -4px 0 12px;
}

.reveal-card.last {
  margin-bottom: 0;
}

.ssh-key-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 12px;
  padding: 12px 16px;
  font-size: 12px;
}

.row input {
  flex: 1;
}

button.active {
  background: var(--color-selected);
}

.custom-field {
  display: grid;
  grid-template-columns: 1fr 1.4fr auto auto;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.concealed {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  white-space: nowrap;
}

.concealed span {
  margin: 0;
  font-size: 12px;
  font-weight: 400;
  color: var(--color-text-secondary);
}

.add-field {
  padding-left: 0;
  color: var(--color-text-muted);
}

.footer {
  margin-top: 4px;
}

.footer .muted {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
}

.passkey-line {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 6px 0;
}

.passkey-note {
  margin: 8px 0 0;
  font-size: 12px;
}
</style>
