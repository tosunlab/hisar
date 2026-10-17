<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from "vue";
import {
  api,
  createEmptyAddress,
  createEmptyAPICredential,
  createEmptyCard,
  createEmptyDatabase,
  createEmptyIdentity,
  createEmptyItem,
  createEmptyServer,
  createEmptySoftwareLicense,
  type CardDescription,
  type ItemDetail,
} from "../api";
import {
  addressCategory,
  allCardBrand,
  allDatabaseEngine,
  apiCredentialCategory,
  creditCardCategory,
  databaseCategory,
  describeAPICredentialExpiry,
  describeServerPort,
  identityCategory,
  formatCardNumberInput,
  formatExpiryInput,
  formatInputAsTyped,
  getItemIcon,
  getWebsiteIconURL,
  isCalendarDate,
  isKnownDatabaseEngine,
  secureNoteCategory,
  serverCategory,
  softwareLicenseCategory,
  sshKeyCategory,
} from "../util";
import { cancelEditing, saveItem, store } from "../store";
import AddressFields from "./AddressFields.vue";
import AppIcon from "./AppIcon.vue";
import IconPicker from "./IconPicker.vue";
import ItemTile from "./ItemTile.vue";
import PasswordGenerator from "./PasswordGenerator.vue";
import StrengthMeter from "./StrengthMeter.vue";

function createDraft(): ItemDetail {
  if (store.editorMode === "edit" && store.selectedItem) {
    const selected = store.selectedItem;
    return {
      ...selected,
      urls: [...selected.urls],
      tags: [...(selected.tags ?? [])],
      fields: selected.fields.map((field) => ({ ...field })),
      passkeys: [...(selected.passkeys ?? [])],
      card: selected.card ? { ...selected.card } : selected.category === creditCardCategory ? createEmptyCard() : selected.card,
      identity: selected.identity ? { ...selected.identity } : selected.category === identityCategory ? createEmptyIdentity() : selected.identity,
      apiCredential: selected.apiCredential
        ? { ...selected.apiCredential }
        : selected.category === apiCredentialCategory
          ? createEmptyAPICredential()
          : selected.apiCredential,
      address: selected.address ? { ...selected.address } : selected.category === addressCategory ? createEmptyAddress() : selected.address,
      softwareLicense: selected.softwareLicense
        ? { ...selected.softwareLicense }
        : selected.category === softwareLicenseCategory
          ? createEmptySoftwareLicense()
          : selected.softwareLicense,
      server: selected.server ? { ...selected.server } : selected.category === serverCategory ? createEmptyServer() : selected.server,
      database: selected.database ? { ...selected.database } : selected.category === databaseCategory ? createEmptyDatabase() : selected.database,
    };
  }
  const empty = createEmptyItem();
  empty.tags = empty.tags ?? [];
  if (store.editorMode === "create-secure-note") {
    empty.category = secureNoteCategory;
  } else if (store.editorMode === "create-credit-card") {
    empty.category = creditCardCategory;
    empty.card = createEmptyCard();
  } else if (store.editorMode === "create-identity") {
    empty.category = identityCategory;
    empty.identity = createEmptyIdentity();
  } else if (store.editorMode === "create-api-credential") {
    empty.category = apiCredentialCategory;
    empty.apiCredential = createEmptyAPICredential();
  } else if (store.editorMode === "create-address") {
    empty.category = addressCategory;
    empty.address = createEmptyAddress();
  } else if (store.editorMode === "create-software-license") {
    empty.category = softwareLicenseCategory;
    empty.softwareLicense = createEmptySoftwareLicense();
  } else if (store.editorMode === "create-server") {
    empty.category = serverCategory;
    empty.server = createEmptyServer();
  } else if (store.editorMode === "create-database") {
    empty.category = databaseCategory;
    empty.database = createEmptyDatabase();
  }
  return empty;
}

const draft = reactive<ItemDetail>(createDraft());
const urlText = ref(draft.urls.join("\n"));
const isPasswordVisible = ref(false);
const isGeneratorOpen = ref(store.editorMode === "create");
const isIconPickerOpen = ref(false);
// websiteIconURL is the website whose icon Automatic shows, from the URLs as typed.
const websiteIconURL = computed(() => getWebsiteIconURL({ category: draft.category, urls: urlText.value.split("\n") }));
const isSSHKey = draft.category === sshKeyCategory;
const isSecureNote = draft.category === secureNoteCategory;
const isCreditCard = draft.category === creditCardCategory;
const isIdentity = draft.category === identityCategory;
const isAPICredential = draft.category === apiCredentialCategory;
const isAddress = draft.category === addressCategory;
const isSoftwareLicense = draft.category === softwareLicenseCategory;
const isServer = draft.category === serverCategory;
const serverPortWarning = computed(() => describeServerPort(draft.server?.port.trim() ?? ""));
// The SSH keys a server may use: the vault's, which the list shows. A linked key that is gone stays chosen as "Missing SSH
// key" until another is picked.
const allSSHKey = computed(() => store.allItem.filter((vaultItem) => vaultItem.category === sshKeyCategory));
const isSSHKeyMissing = computed(() => Boolean(draft.server?.sshKeyId) && !allSSHKey.value.some((key) => key.id === draft.server?.sshKeyId));
const isDatabase = draft.category === databaseCategory;
const databasePortWarning = computed(() => describeServerPort(draft.database?.port.trim() ?? ""));
// The engine select shows a known engine by its name; any other, such as an imported DB2, is Other with its text editable.
const engineChoice = ref(isKnownDatabaseEngine(draft.database?.engine ?? "") ? (draft.database?.engine ?? "") : "other");
watch(engineChoice, (choice) => {
  if (!draft.database) {
    return;
  }
  if (choice !== "other") {
    draft.database.engine = choice;
  } else if (isKnownDatabaseEngine(draft.database.engine)) {
    draft.database.engine = "";
  }
});
const isLicenseKeyVisible = ref(false);
// A purchase date imported in another form, such as 03/09/2024, stays editable as text, as a birth date does.
const isPurchaseDateText = Boolean(draft.softwareLicense?.purchaseDate) && !isCalendarDate(draft.softwareLicense?.purchaseDate ?? "");
const isDocumentNumberVisible = ref(false);
const isCredentialSecretVisible = ref(false);
// An expiry in another form, such as "never", stays editable as text, as a birth date does.
const isExpiryText = Boolean(draft.apiCredential?.expiry) && !isCalendarDate(draft.apiCredential?.expiry ?? "");
// The warning follows the expiry as it is edited. A saved value in another form that the backend still reads as a date,
// such as 2020/01/01, keeps the backend's warning until it changes.
const savedExpiry = draft.apiCredential?.expiry ?? "";
const savedExpiryWarning = draft.apiCredential?.expiryWarning ?? "";
const credentialExpiryWarning = computed(() => {
  const expiry = draft.apiCredential?.expiry ?? "";
  if (!isCalendarDate(expiry) && expiry === savedExpiry) {
    return savedExpiryWarning;
  }
  return describeAPICredentialExpiry(expiry);
});
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

const tagText = ref("");
const tagInput = ref<HTMLInputElement | null>(null);

// addTag turns what was typed into tags, one per comma, skipping blanks and tags the item already has.
function addTag(): void {
  for (const tag of tagText.value.split(",")) {
    const trimmedTag = tag.trim();
    if (trimmedTag !== "" && !draft.tags.includes(trimmedTag)) {
      draft.tags.push(trimmedTag);
    }
  }
  tagText.value = "";
}

function removeTag(tag: string): void {
  draft.tags = draft.tags.filter((candidate) => candidate !== tag);
}

// A comma adds the tag like Enter does; Backspace in the empty field removes the last tag.
function handleTagKey(event: KeyboardEvent): void {
  if (event.key === "," || event.key === "Enter") {
    event.preventDefault();
    addTag();
  } else if (event.key === "Backspace" && tagText.value === "" && draft.tags.length > 0) {
    event.preventDefault();
    draft.tags.pop();
  }
}

async function submitItem(): Promise<void> {
  addTag();
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
      <div class="icon-anchor">
        <button
          type="button"
          class="icon-button"
          aria-label="Change icon"
          title="Change icon"
          :aria-expanded="isIconPickerOpen"
          data-icon-picker-anchor
          data-testid="item-icon-button"
          @click="isIconPickerOpen = !isIconPickerOpen"
        >
          <ItemTile :title="draft.title || '?'" :size="52" :icon="getItemIcon(draft.category)" :item-icon="draft.icon" :websiteURL="websiteIconURL" />
          <span class="icon-badge"><AppIcon name="edit" :size="10" /></span>
        </button>
        <IconPicker
          v-if="isIconPickerOpen"
          v-model="draft.icon"
          :title="draft.title"
          :websiteURL="websiteIconURL"
          @close="isIconPickerOpen = false"
        />
      </div>
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
      <p class="section-label">Billing address</p>
      <AddressFields v-model:old-text="draft.card.billingAddress" :address="draft.card" old-label="Billing address" />
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
      <p class="section-label">Address</p>
      <AddressFields v-model:old-text="draft.identity.address" :address="draft.identity" class="identity-address" />
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

    <section v-if="isAPICredential && draft.apiCredential" class="card editor-card" data-testid="api-credential-editor">
      <div class="row card-row">
        <label>
          <span>Service</span>
          <input v-model="draft.apiCredential.service" name="service" autocomplete="off" placeholder="Stripe" />
        </label>
        <label>
          <span>Key ID</span>
          <input v-model="draft.apiCredential.keyId" name="keyId" class="monospace" autocomplete="off" spellcheck="false" />
        </label>
      </div>
      <label>
        <span>Secret</span>
        <span class="row">
          <input
            v-model="draft.apiCredential.secret"
            name="secret"
            class="monospace"
            :type="isCredentialSecretVisible ? 'text' : 'password'"
            autocomplete="off"
            spellcheck="false"
          />
          <button
            type="button"
            class="icon-button"
            :aria-label="isCredentialSecretVisible ? 'Hide secret' : 'Show secret'"
            @click="isCredentialSecretVisible = !isCredentialSecretVisible"
          >
            <AppIcon :name="isCredentialSecretVisible ? 'eyeOff' : 'eye'" />
          </button>
        </span>
      </label>
      <div class="row card-row">
        <label class="last">
          <span>Host</span>
          <input v-model="draft.apiCredential.host" name="host" autocomplete="off" spellcheck="false" placeholder="api.example.com" />
        </label>
        <label class="last">
          <span>Expiry</span>
          <input v-model="draft.apiCredential.expiry" name="expiry" :type="isExpiryText ? 'text' : 'date'" autocomplete="off" />
          <small v-if="credentialExpiryWarning" class="hint warning-text" data-testid="api-credential-expiry-warning">{{ credentialExpiryWarning }}</small>
        </label>
      </div>
    </section>

    <section v-if="isAddress && draft.address" class="card editor-card" data-testid="address-editor">
      <AddressFields :address="draft.address" />
    </section>

    <section v-if="isSoftwareLicense && draft.softwareLicense" class="card editor-card" data-testid="software-license-editor">
      <div class="row card-row">
        <label>
          <span>Product</span>
          <input v-model="draft.softwareLicense.product" name="product" autocomplete="off" />
        </label>
        <label>
          <span>Version</span>
          <input v-model="draft.softwareLicense.version" name="version" autocomplete="off" spellcheck="false" />
        </label>
      </div>
      <label>
        <span>License key</span>
        <span class="row">
          <input
            v-model="draft.softwareLicense.licenseKey"
            name="licenseKey"
            class="monospace"
            :type="isLicenseKeyVisible ? 'text' : 'password'"
            autocomplete="off"
            spellcheck="false"
          />
          <button
            type="button"
            class="icon-button"
            :aria-label="isLicenseKeyVisible ? 'Hide license key' : 'Show license key'"
            @click="isLicenseKeyVisible = !isLicenseKeyVisible"
          >
            <AppIcon :name="isLicenseKeyVisible ? 'eyeOff' : 'eye'" />
          </button>
        </span>
      </label>
      <div class="row card-row">
        <label>
          <span>Licensed to</span>
          <input v-model="draft.softwareLicense.licensedTo" name="licensedTo" autocomplete="off" />
        </label>
        <label>
          <span>Email</span>
          <!-- Not type="email", as an identity's: the browser would refuse a value it does not take for one address. -->
          <input v-model="draft.softwareLicense.email" name="licenseEmail" autocomplete="off" spellcheck="false" />
        </label>
      </div>
      <div class="row card-row">
        <label class="last">
          <span>Purchase date</span>
          <input v-model="draft.softwareLicense.purchaseDate" name="purchaseDate" :type="isPurchaseDateText ? 'text' : 'date'" autocomplete="off" />
        </label>
        <label class="last">
          <span>Download URL</span>
          <input v-model="draft.softwareLicense.downloadUrl" name="downloadUrl" autocomplete="off" spellcheck="false" placeholder="https://example.com/download" />
        </label>
      </div>
    </section>

    <section v-if="isServer && draft.server" class="card editor-card" data-testid="server-editor">
      <div class="row card-row">
        <label>
          <span>Host</span>
          <input v-model="draft.server.host" name="host" autocomplete="off" spellcheck="false" placeholder="server.example.com" />
        </label>
        <label>
          <span>Port</span>
          <input v-model="draft.server.port" name="port" inputmode="numeric" autocomplete="off" placeholder="22" />
          <small v-if="serverPortWarning" class="hint warning-text" data-testid="server-port-warning">{{ serverPortWarning }}</small>
        </label>
      </div>
      <label class="last">
        <span>SSH key</span>
        <select v-model="draft.server.sshKeyId" name="sshKey">
          <option value="">None</option>
          <option v-if="isSSHKeyMissing" :value="draft.server.sshKeyId">Missing SSH key</option>
          <option v-for="key in allSSHKey" :key="key.id" :value="key.id">{{ key.title }}</option>
        </select>
        <small class="hint muted">An SSH key from this vault, so everyone who sees the server can use it.</small>
      </label>
    </section>

    <section v-if="isDatabase && draft.database" class="card editor-card" data-testid="database-editor">
      <div class="row card-row">
        <label>
          <span>Engine</span>
          <select v-model="engineChoice" name="engine">
            <option v-for="engine in allDatabaseEngine" :key="engine.id" :value="engine.id">{{ engine.name }}</option>
            <option value="other">Other</option>
          </select>
        </label>
        <label v-if="engineChoice === 'other'">
          <span>Other engine</span>
          <input v-model="draft.database.engine" name="otherEngine" autocomplete="off" spellcheck="false" placeholder="DB2" />
        </label>
      </div>
      <div class="row card-row">
        <label>
          <span>Host</span>
          <input v-model="draft.database.host" name="host" autocomplete="off" spellcheck="false" placeholder="db.example.com" />
        </label>
        <label>
          <span>Port</span>
          <input v-model="draft.database.port" name="port" inputmode="numeric" autocomplete="off" placeholder="5432" />
          <small v-if="databasePortWarning" class="hint warning-text" data-testid="database-port-warning">{{ databasePortWarning }}</small>
        </label>
      </div>
      <label>
        <span>Database</span>
        <input v-model="draft.database.name" name="databaseName" autocomplete="off" spellcheck="false" placeholder="app" />
      </label>
      <label class="last">
        <span>Options</span>
        <input v-model="draft.database.options" name="options" autocomplete="off" spellcheck="false" placeholder="sslmode=require" />
        <small v-if="engineChoice === 'other'" class="hint muted">Hisar builds connection strings only for the engines listed.</small>
      </label>
    </section>

    <p v-if="isSSHKey" class="card ssh-key-note muted" data-testid="ssh-key-note">
      <AppIcon name="key" :size="14" /> The key itself cannot be changed. To replace it, add a new SSH key.
    </p>

    <section v-if="!isSSHKey && !isSecureNote && !isCreditCard && !isIdentity && !isAPICredential && !isAddress && !isSoftwareLicense"
      class="card editor-card"
    >
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
      <label v-if="!isDatabase">
        <span>{{ isServer ? "Admin console URL" : "Websites" }}</span>
        <textarea
          v-model="urlText"
          name="urls"
          rows="2"
          :placeholder="isServer ? 'https://console.example.com' : 'https://example.com (one per line)'"
        ></textarea>
      </label>
      <label v-if="!isDatabase" class="last">
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

    <section class="card editor-card" data-testid="tag-editor">
      <label class="last" for="tagInput"><span>Tags</span></label>
      <div class="tag-field" @click="tagInput?.focus()">
        <span v-for="tag in draft.tags" :key="tag" class="tag-chip" :data-testid="`draft-tag-${tag}`">
          <AppIcon name="tag" :size="11" /> {{ tag }}
          <button type="button" class="ghost icon-button" :aria-label="`Remove tag ${tag}`" @click.stop="removeTag(tag)"><AppIcon name="close" :size="11" /></button>
        </span>
        <input
          id="tagInput"
          ref="tagInput"
          v-model="tagText"
          name="tags"
          autocomplete="off"
          spellcheck="false"
          :placeholder="draft.tags.length === 0 ? 'Add tags, separated by commas' : 'Add a tag'"
          @keydown="handleTagKey"
          @blur="addTag"
        />
      </div>
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

.icon-anchor {
  position: relative;
  flex: none;
}

.icon-button {
  position: relative;
  display: inline-flex;
  padding: 0;
  border: none;
  border-radius: 25%;
  background: none;
}

.icon-badge {
  position: absolute;
  right: -4px;
  bottom: -4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: 1px solid var(--color-border-strong);
  border-radius: 50%;
  background: var(--color-surface-raised);
  color: var(--color-text-secondary);
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

.identity-address {
  margin-bottom: 12px;
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

.tag-field {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  cursor: text;
}

.tag-field:focus-within {
  border-color: var(--color-focus);
  box-shadow: 0 0 0 3px var(--color-focus-ring);
}

.tag-field input {
  flex: 1;
  min-width: 140px;
  padding: 3px 4px;
  border: none;
  background: transparent;
  box-shadow: none;
}

.tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 2px 1px 8px;
  border-radius: 10px;
  background: var(--color-selected);
  font-size: 12px;
}

.tag-chip .icon-button {
  padding: 2px;
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
