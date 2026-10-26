<script setup lang="ts">
import { computed, ref, watch } from "vue";
import {
  convertToSecureNote,
  copyField,
  copySecretReference,
  copyText,
  duplicateItem,
  getAllShareLinkForItem,
  hasShareableField,
  openItemShareLinkDialog,
  openWebsite,
  selectItem,
  selectTag,
  startEditingItem,
  store,
  toggleSelectedItemFavorite,
} from "../store";
import { bindCopyRow, bindCopyTarget, bindFieldRow } from "../copyrow";
import {
  addressCategory,
  addressFieldName,
  allAddressKey,
  apiCredentialCategory,
  apiCredentialFieldName,
  cardFieldName,
  creditCardCategory,
  databaseCategory,
  databaseFieldName,
  formatAddressSummary,
  formatAPICredentialSummary,
  formatCardSummary,
  formatDatabaseSummary,
  formatIdentitySummary,
  formatKeyType,
  formatServerAddress,
  formatServerSummary,
  formatSoftwareLicenseSummary,
  getDomain,
  getItemIcon,
  getWebsiteIconURL,
  identityCategory,
  identityFieldName,
  isMacPlatform,
  secureNoteCategory,
  serverCategory,
  serverFieldName,
  softwareLicenseCategory,
  softwareLicenseFieldName,
  sshKeyCategory,
} from "../util";
import AppIcon from "./AppIcon.vue";
import CommitSigningPanel from "./CommitSigningPanel.vue";
import ConfirmDeleteDialog from "./ConfirmDeleteDialog.vue";
import ContextMenu, { type ContextMenuAction } from "./ContextMenu.vue";
import ItemTile from "./ItemTile.vue";
import MoveItemDialog from "./MoveItemDialog.vue";
import OneTimeCodeField from "./OneTimeCodeField.vue";
import PasskeyList from "./PasskeyList.vue";
import SecretReferenceButton from "./SecretReferenceButton.vue";
import SharedLinksList from "./SharedLinksList.vue";
import StrengthMeter from "./StrengthMeter.vue";

// vaultType is the vault's type: personal, shared or organization. Only a shared vault's items say "shared".
defineProps<{ canEdit: boolean; vaultName: string; vaultType: string }>();

const shortcutModifier = isMacPlatform() ? "⌘" : "Ctrl+";
const isPasswordRevealed = ref(false);
const allRevealedFieldName = ref<string[]>([]);
// itemMenu is the header's ⋯ menu, opened below its button; dialog is the Move or Delete dialog one of its actions opens.
const itemMenu = ref<{ x: number; y: number; trigger: HTMLButtonElement } | null>(null);
const dialog = ref<"move" | "delete" | null>(null);
const isSSHKey = computed(() => store.selectedItem?.category === sshKeyCategory);
const isSecureNote = computed(() => store.selectedItem?.category === secureNoteCategory);
const isCreditCard = computed(() => store.selectedItem?.category === creditCardCategory);
const isIdentity = computed(() => store.selectedItem?.category === identityCategory);
const isAPICredential = computed(() => store.selectedItem?.category === apiCredentialCategory);
const isAddress = computed(() => store.selectedItem?.category === addressCategory);
const isSoftwareLicense = computed(() => store.selectedItem?.category === softwareLicenseCategory);
const isServer = computed(() => store.selectedItem?.category === serverCategory);
const server = computed(() => (isServer.value ? store.selectedItem?.server : undefined));
const isDatabase = computed(() => store.selectedItem?.category === databaseCategory);
const database = computed(() => (isDatabase.value ? store.selectedItem?.database : undefined));
const isConnectionStringRevealed = ref(false);
// websiteLabel names a URL row: a server's is its admin console.
const websiteLabel = computed(() => (isServer.value ? "Admin console" : "Website"));
// A database's values before its login data: the engine by its name, the host, port and database name.
const allDatabaseRow = computed(() => {
  const value = database.value;
  if (!value) {
    return [];
  }
  return [
    { name: databaseFieldName.engine, value: value.engineName, warning: "" },
    { name: databaseFieldName.host, value: value.host, warning: "" },
    { name: databaseFieldName.port, value: value.port, warning: value.portWarning },
    { name: databaseFieldName.name, value: value.name, warning: "" },
  ].filter((row) => row.value !== "");
});

// NamedRow: isLink rows open in the browser, as websites do.
type NamedRow = { name: string; value: string; concealed: boolean; masked: string; warning: string; isLink?: boolean };

// buildAddressRows lists an address's values as rows, as an address item, an identity and a card keep them.
function buildAddressRows(address: Record<(typeof allAddressKey)[number], string>): NamedRow[] {
  return allAddressKey.map((key) => ({ name: addressFieldName[key], value: address[key], concealed: false, masked: "", warning: "" }));
}

// The card's values as rows; the number shows only its last four digits until revealed.
const allCardRow = computed(() => {
  const card = store.selectedItem?.card;
  if (!card) {
    return [];
  }
  return [
    { name: cardFieldName.cardholder, value: card.cardholder, concealed: false, masked: "", warning: "" },
    { name: cardFieldName.type, value: card.type, concealed: false, masked: "", warning: "" },
    { name: cardFieldName.number, value: card.number, concealed: true, masked: card.last4 ? `•••• •••• •••• ${card.last4}` : "", warning: card.numberWarning },
    { name: cardFieldName.expiry, value: card.expiry, concealed: false, masked: "", warning: card.expiryWarning },
    { name: cardFieldName.securityCode, value: card.securityCode, concealed: true, masked: "", warning: "" },
    { name: cardFieldName.pin, value: card.pin, concealed: true, masked: "", warning: "" },
    ...buildAddressRows(card),
    { name: cardFieldName.billingAddress, value: card.billingAddress, concealed: false, masked: "", warning: "" },
  ].filter((row) => row.value !== "");
});
// An identity's values as rows; the document numbers are masked until revealed.
const allIdentityRow = computed(() => {
  const identity = store.selectedItem?.identity;
  if (!identity) {
    return [];
  }
  const buildRows = (allKey: (keyof typeof identityFieldName)[]): NamedRow[] =>
    allKey.map((key) => ({
      name: identityFieldName[key],
      value: identity[key],
      concealed: key === "passportNumber" || key === "driversLicenseNumber" || key === "nationalIdNumber",
      masked: "",
      warning: "",
    }));
  // The address comes after the phone, as core/item stores it.
  return [
    ...buildRows(["firstName", "middleName", "lastName", "birthDate", "email", "phone"]),
    ...buildAddressRows(identity),
    ...buildRows(["address", "passportNumber", "driversLicenseNumber", "nationalIdNumber"]),
  ].filter((row) => row.value !== "");
});
// An API credential's values as rows; the secret is masked until revealed and the expiry warns once it has passed.
const allAPICredentialRow = computed(() => {
  const credential = store.selectedItem?.apiCredential;
  if (!credential) {
    return [];
  }
  return [
    { name: apiCredentialFieldName.service, value: credential.service, concealed: false, masked: "", warning: "" },
    { name: apiCredentialFieldName.keyId, value: credential.keyId, concealed: false, masked: "", warning: "" },
    { name: apiCredentialFieldName.secret, value: credential.secret, concealed: true, masked: "", warning: "" },
    { name: apiCredentialFieldName.host, value: credential.host, concealed: false, masked: "", warning: "" },
    { name: apiCredentialFieldName.expiry, value: credential.expiry, concealed: false, masked: "", warning: credential.expiryWarning },
  ].filter((row) => row.value !== "");
});
const allAddressRow = computed(() => (store.selectedItem?.address ? buildAddressRows(store.selectedItem.address).filter((row) => row.value !== "") : []));
// A software license's values as rows; the license key is masked until revealed and the download URL opens.
const allSoftwareLicenseRow = computed(() => {
  const license = store.selectedItem?.softwareLicense;
  if (!license) {
    return [];
  }
  const rows: NamedRow[] = [
    { name: softwareLicenseFieldName.product, value: license.product, concealed: false, masked: "", warning: "" },
    { name: softwareLicenseFieldName.version, value: license.version, concealed: false, masked: "", warning: "" },
    { name: softwareLicenseFieldName.licenseKey, value: license.licenseKey, concealed: true, masked: "", warning: "" },
    { name: softwareLicenseFieldName.licensedTo, value: license.licensedTo, concealed: false, masked: "", warning: "" },
    { name: softwareLicenseFieldName.email, value: license.email, concealed: false, masked: "", warning: "" },
    { name: softwareLicenseFieldName.purchaseDate, value: license.purchaseDate, concealed: false, masked: "", warning: "" },
    { name: softwareLicenseFieldName.downloadUrl, value: license.downloadUrl, concealed: false, masked: "", warning: "", isLink: true },
  ];
  return rows.filter((row) => row.value !== "");
});
// The rows of a type whose values are named fields, what its section calls it when it has none, and the section's test
// id: a card's, an identity's, an API credential's, an address's or a software license's.
const namedType = computed(() => {
  if (isSoftwareLicense.value) {
    return { allRow: allSoftwareLicenseRow.value, name: "software license", testID: "software-license-fields" };
  }
  if (isIdentity.value) {
    return { allRow: allIdentityRow.value, name: "identity", testID: "identity-fields" };
  }
  if (isAPICredential.value) {
    return { allRow: allAPICredentialRow.value, name: "API credential", testID: "api-credential-fields" };
  }
  if (isAddress.value) {
    return { allRow: allAddressRow.value, name: "address", testID: "address-fields" };
  }
  return { allRow: allCardRow.value, name: "card", testID: "credit-card-fields" };
});
const allNamedRow = computed((): NamedRow[] => namedType.value.allRow);
const allMultilineName: string[] = [addressFieldName.street, cardFieldName.billingAddress, identityFieldName.address];
const isShareable = computed(() => store.selectedItem !== null && hasShareableField(store.selectedItem));
const allItemShareLink = computed(() => (store.selectedItem ? getAllShareLinkForItem(store.selectedVaultID, store.selectedItem.id) : []));

watch(
  () => store.selectedItem?.id,
  () => {
    isPasswordRevealed.value = false;
    isConnectionStringRevealed.value = false;
    allRevealedFieldName.value = [];
    itemMenu.value = null;
    dialog.value = null;
    fieldMenu.value = null;
  },
);

function toggleFieldReveal(fieldName: string): void {
  allRevealedFieldName.value = allRevealedFieldName.value.includes(fieldName)
    ? allRevealedFieldName.value.filter((name) => name !== fieldName)
    : [...allRevealedFieldName.value, fieldName];
}

// buildItemMenuActions lists what the ⋯ menu offers, as the item's right-click menu in the list does: Duplicate (not for an
// SSH key, whose private key stays one), Move to… when another vault takes items, and Delete… last.
function buildItemMenuActions(): ContextMenuAction[] {
  const item = store.selectedItem;
  if (!item) {
    return [];
  }
  const vaultID = store.selectedVaultID;
  const hasMoveTarget = store.allVault.some((vault) => vault.id !== vaultID && vault.role !== "reader");
  const allAction: (ContextMenuAction | null)[] = [
    item.category !== sshKeyCategory ? { id: "duplicate", label: "Duplicate", icon: "duplicate", run: () => duplicateItem(vaultID, item.id) } : null,
    hasMoveTarget ? { id: "move", label: "Move to…", icon: "move", run: () => (dialog.value = "move") } : null,
    { id: "delete", label: "Delete…", icon: "trash", isDanger: true, run: () => (dialog.value = "delete") },
  ];
  return allAction.filter((action): action is ContextMenuAction => action !== null);
}

function openItemMenu(event: MouseEvent): void {
  const trigger = event.currentTarget as HTMLButtonElement;
  const bounds = trigger.getBoundingClientRect();
  itemMenu.value = { x: Math.max(8, bounds.right - 200), y: bounds.bottom + 6, trigger };
}

function closeItemMenu(): void {
  itemMenu.value?.trigger.focus();
  itemMenu.value = null;
}

// fieldMenu is the right-click menu of a field row. It offers the row's own buttons, by their labels, so it always matches
// what the row can do: copy, reveal or hide, copy the secret reference, open. A row is a .field with its .field-actions, or
// a block marked data-field-menu-row, such as an SSH key's public key or a note, whose buttons sit in data-field-menu-actions.
const fieldMenu = ref<{ label: string; x: number; y: number; allAction: ContextMenuAction[] } | null>(null);

function chooseFieldMenuIcon(label: string): ContextMenuAction["icon"] {
  if (label.startsWith("Copy secret reference")) {
    return "terminal";
  }
  if (label.startsWith("Copy")) {
    return "copy";
  }
  if (label.startsWith("Reveal")) {
    return "eye";
  }
  if (label.startsWith("Hide")) {
    return "eyeOff";
  }
  return label.startsWith("Open") ? "external" : "field";
}

function openFieldMenu(event: MouseEvent): void {
  const row = (event.target as HTMLElement).closest<HTMLElement>(".field, [data-field-menu-row]");
  // Text selected in the row keeps the system's menu, with its Copy.
  const selection = window.getSelection();
  if (!row || (selection && !selection.isCollapsed && row.contains(selection.anchorNode))) {
    return;
  }
  const allButton = [...row.querySelectorAll<HTMLButtonElement>(".field-actions button, [data-field-menu-actions] button")].filter(
    (button) => !button.disabled && button.getAttribute("aria-label"),
  );
  if (allButton.length === 0) {
    return;
  }
  event.preventDefault();
  const allAction = allButton.map((button) => {
    const label = button.getAttribute("aria-label") ?? "";
    return { id: label.toLowerCase().replace(/[^a-z0-9]+/g, "-"), label, icon: chooseFieldMenuIcon(label), run: () => button.click() };
  });
  // Only secrets show a secret reference button; any other field offers its reference here.
  const referenceField = row.dataset.referenceField;
  const referenceLabel = `Copy secret reference for ${referenceField}`;
  if (referenceField && !allAction.some((action) => action.label === referenceLabel)) {
    const id = referenceLabel.toLowerCase().replace(/[^a-z0-9]+/g, "-");
    allAction.push({ id, label: referenceLabel, icon: "terminal", run: () => copySecretReference(referenceField) });
  }
  const label = row.querySelector(".field-label, .section-label")?.textContent?.trim() || "Field";
  fieldMenu.value = { label, x: event.clientX, y: event.clientY, allAction };
}

function maskSecret(secret: string): string {
  return "•".repeat(Math.min(Math.max(secret.length, 10), 20));
}
</script>

<template>
  <article v-if="store.selectedItem" class="item-detail" @contextmenu="openFieldMenu">
    <header>
      <ItemTile
        :title="store.selectedItem.title"
        :size="52"
        :icon="getItemIcon(store.selectedItem.category)"
        :item-icon="store.selectedItem.icon"
        :websiteURL="getWebsiteIconURL(store.selectedItem)"
      />
      <div class="heading">
        <h2>{{ store.selectedItem.title }}</h2>
        <div class="meta">
          <span class="badge" data-testid="item-vault"><AppIcon :name="vaultType === 'personal' ? 'user' : 'users'" :size="12" /> {{ vaultName }}</span>
          <span v-if="vaultType === 'shared'" class="badge" data-testid="item-shared">Shared</span>
          <span v-if="isSSHKey" class="muted">SSH key</span>
          <span v-else-if="isSecureNote" class="muted">Secure note</span>
          <span v-else-if="isCreditCard" class="muted" data-testid="card-summary">{{ formatCardSummary(store.selectedItem.card) }}</span>
          <span v-else-if="isIdentity" class="muted" data-testid="identity-summary">{{ formatIdentitySummary(store.selectedItem.identity) }}</span>
          <span v-else-if="isAPICredential" class="muted" data-testid="api-credential-summary">{{ formatAPICredentialSummary(store.selectedItem.apiCredential) }}</span>
          <span v-else-if="isAddress" class="muted" data-testid="address-summary">{{ formatAddressSummary(store.selectedItem.address) }}</span>
          <span v-else-if="isSoftwareLicense" class="muted" data-testid="software-license-summary">{{
            formatSoftwareLicenseSummary(store.selectedItem.softwareLicense)
          }}</span>
          <span v-else-if="isServer" class="muted" data-testid="server-summary">{{
            formatServerSummary(store.selectedItem.server, store.selectedItem.username)
          }}</span>
          <span v-else-if="isDatabase" class="muted" data-testid="database-summary">{{ formatDatabaseSummary(store.selectedItem.database) }}</span>
          <span v-else-if="store.selectedItem.urls[0]" class="muted">{{ getDomain(store.selectedItem.urls[0]) }}</span>
        </div>
        <div v-if="(store.selectedItem.tags ?? []).length > 0" class="tags" data-testid="item-tags">
          <button
            v-for="tag in store.selectedItem.tags ?? []"
            :key="tag"
            class="tag-chip"
            :class="{ selected: tag === store.selectedTag }"
            :title="tag === store.selectedTag ? 'Show all items' : `Show items tagged ${tag}`"
            @click="selectTag(tag)"
          >
            <AppIcon name="tag" :size="11" /> {{ tag }}
          </button>
        </div>
      </div>
      <div class="header-actions">
        <button
          class="ghost icon-button favorite-button"
          :class="{ 'is-favorite': store.selectedItem.isFavorite }"
          :aria-label="store.selectedItem.isFavorite ? 'Remove from Favorites' : 'Add to Favorites'"
          :aria-pressed="store.selectedItem.isFavorite"
          :title="`${store.selectedItem.isFavorite ? 'Remove from Favorites' : 'Add to Favorites'} (${shortcutModifier}D)`"
          data-testid="favorite-toggle"
          @click="toggleSelectedItemFavorite"
        >
          <AppIcon :name="store.selectedItem.isFavorite ? 'starFilled' : 'star'" />
        </button>
        <button v-if="isShareable" title="Share with anyone through an end-to-end encrypted link" data-testid="share-link" @click="openItemShareLinkDialog">
          <AppIcon name="link" /> Share link…
        </button>
        <template v-if="canEdit">
          <button @click="startEditingItem"><AppIcon name="edit" /> Edit</button>
          <button
            class="ghost icon-button"
            aria-label="Item actions"
            title="Item actions"
            aria-haspopup="menu"
            :aria-expanded="itemMenu !== null"
            data-testid="item-actions"
            @pointerdown.stop
            @click="itemMenu ? (itemMenu = null) : openItemMenu($event)"
          >
            <AppIcon name="more" />
          </button>
        </template>
      </div>
    </header>

    <template v-if="isSSHKey">
      <section class="card field-card" data-testid="ssh-key-fields">
        <div class="field">
          <span class="field-label">Key type</span>
          <span class="field-value">{{ formatKeyType(store.selectedItem.keyType) }}</span>
          <span class="field-actions"></span>
        </div>
        <div class="field" v-bind="bindFieldRow('fingerprint', 'Fingerprint')">
          <div class="field-copy-target" v-bind="bindCopyTarget()">
          <span class="copy-prefix">Copy </span><span class="field-label">Fingerprint</span>
          <span class="field-value monospace" data-testid="fingerprint">{{ store.selectedItem.fingerprint }}</span>
          </div>
          <span class="field-actions">
            <button class="ghost icon-button" aria-label="Copy fingerprint" title="Copy" @click="copyField('fingerprint', 'Fingerprint')"><AppIcon name="copy" /></button>
          </span>
        </div>
        <div class="field">
          <span class="field-label">Private key</span>
          <span class="field-value muted">Never leaves Hisar; the SSH agent uses it</span>
          <span class="field-actions"></span>
        </div>
        <div class="public-key-field copy-block" data-field-menu-row v-bind="bindFieldRow('public key', 'Public key')">
          <div class="block-copy-target" v-bind="bindCopyTarget()">
            <span class="copy-prefix">Copy </span><span class="field-label">Public key</span>
            <p class="public-key monospace" data-testid="public-key">{{ store.selectedItem.publicKey }}</p>
          </div>
          <div class="row field-actions" data-field-menu-actions>
            <button data-testid="copy-public-key" aria-label="Copy public key" @click="copyField('public key', 'Public key')"><AppIcon name="copy" /> Copy public key</button>
            <SecretReferenceButton field-name="public key" label="public key" />
          </div>
        </div>
      </section>
      <CommitSigningPanel v-if="store.selectedItem.publicKey" :public-key="store.selectedItem.publicKey" />
    </template>

    <section
      v-else-if="isCreditCard || isIdentity || isAPICredential || isAddress || isSoftwareLicense"
      class="card field-card"
      :data-testid="namedType.testID"
    >
      <div v-for="row in allNamedRow" :key="row.name" class="field" v-bind="bindFieldRow(row.name, row.name)">
        <div class="field-copy-target" v-bind="bindCopyTarget()">
        <span class="copy-prefix">Copy </span><span class="field-label">{{ row.name }}</span>
        <span class="field-value">
          <span
            :class="{ monospace: row.concealed, 'multiline-value': allMultilineName.includes(row.name) }"
            :data-testid="`card-${row.name}`"
          >
            {{ row.concealed && !allRevealedFieldName.includes(row.name) ? row.masked || maskSecret(row.value) : row.value }}
          </span>
          <small v-if="row.warning" class="field-warning" :data-testid="`card-warning-${row.name}`">{{ row.warning }}</small>
        </span>
        </div>
        <span class="field-actions">
          <button
            v-if="row.concealed"
            class="ghost icon-button"
            :aria-label="`${allRevealedFieldName.includes(row.name) ? 'Hide' : 'Reveal'} ${row.name}`"
            @click="toggleFieldReveal(row.name)"
          >
            <AppIcon :name="allRevealedFieldName.includes(row.name) ? 'eyeOff' : 'eye'" />
          </button>
          <button v-if="row.isLink" class="ghost icon-button" :aria-label="`Open ${row.name}`" title="Open in browser" @click="openWebsite(row.value)">
            <AppIcon name="external" />
          </button>
          <button class="ghost icon-button" :aria-label="`Copy ${row.name}`" title="Copy" @click="copyField(row.name, row.name)"><AppIcon name="copy" /></button>
          <SecretReferenceButton v-if="row.concealed" :field-name="row.name" :label="row.name" />
        </span>
      </div>
      <div v-for="field in store.selectedItem.fields" :key="field.name" class="field" v-bind="bindFieldRow(field.name, field.name)">
        <div class="field-copy-target" v-bind="bindCopyTarget()">
        <span class="copy-prefix">Copy </span><span class="field-label">{{ field.name }}</span>
        <span class="field-value" :class="{ monospace: field.concealed }">
          {{ field.concealed && !allRevealedFieldName.includes(field.name) ? maskSecret(field.value) : field.value }}
        </span>
        </div>
        <span class="field-actions">
          <button
            v-if="field.concealed"
            class="ghost icon-button"
            :aria-label="`${allRevealedFieldName.includes(field.name) ? 'Hide' : 'Reveal'} ${field.name}`"
            @click="toggleFieldReveal(field.name)"
          >
            <AppIcon :name="allRevealedFieldName.includes(field.name) ? 'eyeOff' : 'eye'" />
          </button>
          <button class="ghost icon-button" :aria-label="`Copy ${field.name}`" title="Copy" @click="copyField(field.name, field.name)"><AppIcon name="copy" /></button>
          <SecretReferenceButton v-if="field.concealed" :field-name="field.name" :label="field.name" />
        </span>
      </div>
      <p v-if="allNamedRow.length === 0 && store.selectedItem.fields.length === 0" class="muted no-fields">
        This {{ namedType.name }} has no details yet.
      </p>
    </section>

    <template v-else-if="isSecureNote">
      <section
        class="card notes-card secure-note copy-block"
        data-testid="secure-note-notes"
        data-field-menu-row
        v-bind="store.selectedItem.notes ? bindFieldRow('notes', 'Notes') : {}"
      >
        <div class="block-copy-target" v-bind="store.selectedItem.notes ? bindCopyTarget() : {}">
          <span v-if="store.selectedItem.notes" class="copy-prefix">Copy </span><p class="section-label">Notes</p>
          <p v-if="store.selectedItem.notes" class="notes">{{ store.selectedItem.notes }}</p>
          <p v-else class="muted no-fields">This note is empty.</p>
        </div>
        <div v-if="store.selectedItem.notes" class="row field-actions" data-field-menu-actions>
          <button data-testid="copy-notes" aria-label="Copy notes" @click="copyField('notes', 'Notes')"><AppIcon name="copy" /> Copy</button>
          <SecretReferenceButton field-name="notes" label="notes" />
        </div>
      </section>
      <section v-if="store.selectedItem.fields.length > 0" class="card field-card note-fields">
        <div v-for="field in store.selectedItem.fields" :key="field.name" class="field" v-bind="bindFieldRow(field.name, field.name)">
          <div class="field-copy-target" v-bind="bindCopyTarget()">
          <span class="copy-prefix">Copy </span><span class="field-label">{{ field.name }}</span>
          <span class="field-value" :class="{ monospace: field.concealed }">
            {{ field.concealed && !allRevealedFieldName.includes(field.name) ? maskSecret(field.value) : field.value }}
          </span>
          </div>
          <span class="field-actions">
            <button
              v-if="field.concealed"
              class="ghost icon-button"
              :aria-label="`${allRevealedFieldName.includes(field.name) ? 'Hide' : 'Reveal'} ${field.name}`"
              @click="toggleFieldReveal(field.name)"
            >
              <AppIcon :name="allRevealedFieldName.includes(field.name) ? 'eyeOff' : 'eye'" />
            </button>
            <button class="ghost icon-button" :aria-label="`Copy ${field.name}`" title="Copy" @click="copyField(field.name, field.name)"><AppIcon name="copy" /></button>
            <SecretReferenceButton v-if="field.concealed" :field-name="field.name" :label="field.name" />
          </span>
        </div>
      </section>
    </template>

    <section v-else class="card field-card">
      <template v-if="server">
        <div v-if="server.host" class="field" v-bind="bindFieldRow(serverFieldName.host, serverFieldName.host)">
          <div class="field-copy-target" v-bind="bindCopyTarget()">
          <span class="copy-prefix">Copy </span><span class="field-label">{{ serverFieldName.host }}</span>
          <span class="field-value monospace" data-testid="server-host">{{ formatServerAddress(server) }}</span>
          </div>
          <span class="field-actions">
            <button class="ghost icon-button" aria-label="Copy Host" title="Copy" @click="copyField(serverFieldName.host, serverFieldName.host)"><AppIcon name="copy" /></button>
          </span>
        </div>
        <div v-if="server.port" class="field" v-bind="bindFieldRow(serverFieldName.port, serverFieldName.port)">
          <div class="field-copy-target" v-bind="bindCopyTarget()">
          <span class="copy-prefix">Copy </span><span class="field-label">{{ serverFieldName.port }}</span>
          <span class="field-value">
            <span data-testid="server-port">{{ server.port }}</span>
            <small v-if="server.portWarning" class="field-warning" data-testid="server-port-warning">{{ server.portWarning }}</small>
          </span>
          </div>
          <span class="field-actions">
            <button class="ghost icon-button" aria-label="Copy Port" title="Copy" @click="copyField(serverFieldName.port, serverFieldName.port)"><AppIcon name="copy" /></button>
          </span>
        </div>
        <div v-if="server.sshKeyId" class="field" data-testid="server-ssh-key">
          <span class="field-label">{{ serverFieldName.sshKey }}</span>
          <span v-if="server.sshKeyTitle" class="field-value">
            <span>{{ server.sshKeyTitle }}</span>
            <small class="muted monospace">{{ server.sshKeyFingerprint }}</small>
          </span>
          <span v-else class="field-value muted">Missing SSH key</span>
          <span class="field-actions">
            <button v-if="server.sshKeyTitle" class="ghost icon-button" aria-label="Open SSH key" title="Open" @click="selectItem(server.sshKeyId)">
              <AppIcon name="key" />
            </button>
          </span>
        </div>
      </template>

      <div v-for="row in allDatabaseRow" :key="row.name" class="field" v-bind="bindFieldRow(row.name, row.name)">
        <div class="field-copy-target" v-bind="bindCopyTarget()">
        <span class="copy-prefix">Copy </span><span class="field-label">{{ row.name }}</span>
        <span class="field-value">
          <span :data-testid="`database-${row.name}`">{{ row.value }}</span>
          <small v-if="row.warning" class="field-warning" data-testid="database-port-warning">{{ row.warning }}</small>
        </span>
        </div>
        <span class="field-actions">
          <button class="ghost icon-button" :aria-label="`Copy ${row.name}`" title="Copy" @click="copyField(row.name, row.name)"><AppIcon name="copy" /></button>
        </span>
      </div>

      <div v-if="store.selectedItem.username" class="field" v-bind="bindFieldRow('username', 'Username')">
        <div class="field-copy-target" v-bind="bindCopyTarget()">
        <span class="copy-prefix">Copy </span><span class="field-label">Username</span>
        <span class="field-value">{{ store.selectedItem.username }}</span>
        </div>
        <span class="field-actions">
          <button class="ghost icon-button" aria-label="Copy username" title="Copy" @click="copyField('username', 'Username')"><AppIcon name="copy" /></button>
        </span>
      </div>

      <div v-if="store.selectedItem.password" class="field" v-bind="bindFieldRow('password', 'Password')">
        <div class="field-copy-target" v-bind="bindCopyTarget()">
        <span class="copy-prefix">Copy </span><span class="field-label">Password</span>
        <span class="field-value">
          <span class="monospace secret" data-testid="password-value">
            {{ isPasswordRevealed ? store.selectedItem.password : maskSecret(store.selectedItem.password) }}
          </span>
          <StrengthMeter :password="store.selectedItem.password" />
        </span>
        </div>
        <span class="field-actions">
          <button
            class="ghost icon-button"
            :aria-label="isPasswordRevealed ? 'Hide password' : 'Reveal password'"
            :title="isPasswordRevealed ? 'Hide' : 'Reveal'"
            @click="isPasswordRevealed = !isPasswordRevealed"
          >
            <AppIcon :name="isPasswordRevealed ? 'eyeOff' : 'eye'" />
          </button>
          <button class="ghost icon-button" aria-label="Copy password" title="Copy" @click="copyField('password', 'Password')"><AppIcon name="copy" /></button>
          <SecretReferenceButton field-name="password" label="password" />
        </span>
      </div>

      <template v-if="database">
        <div v-if="database.options" class="field" v-bind="bindFieldRow(databaseFieldName.options, databaseFieldName.options)">
          <div class="field-copy-target" v-bind="bindCopyTarget()">
          <span class="copy-prefix">Copy </span><span class="field-label">{{ databaseFieldName.options }}</span>
          <span class="field-value monospace" data-testid="database-options">{{ database.options }}</span>
          </div>
          <span class="field-actions">
            <button class="ghost icon-button" aria-label="Copy Options" title="Copy" @click="copyField(databaseFieldName.options, databaseFieldName.options)"><AppIcon name="copy" /></button>
          </span>
        </div>
        <div
          v-if="database.connectionString"
          class="field"
          data-testid="database-connection-string"
          v-bind="bindFieldRow('connection string', 'Connection string')"
        >
          <div class="field-copy-target" v-bind="bindCopyTarget()">
          <span class="copy-prefix">Copy </span><span class="field-label">Connection string</span>
          <span class="field-value monospace secret" data-testid="connection-string-value">
            {{ isConnectionStringRevealed ? database.connectionString : maskSecret(database.connectionString) }}
          </span>
          </div>
          <span class="field-actions">
            <button
              class="ghost icon-button"
              :aria-label="isConnectionStringRevealed ? 'Hide connection string' : 'Reveal connection string'"
              :title="isConnectionStringRevealed ? 'Hide' : 'Reveal'"
              @click="isConnectionStringRevealed = !isConnectionStringRevealed"
            >
              <AppIcon :name="isConnectionStringRevealed ? 'eyeOff' : 'eye'" />
            </button>
            <button class="ghost icon-button" aria-label="Copy connection string" title="Copy" @click="copyField('connection string', 'Connection string')">
              <AppIcon name="copy" />
            </button>
            <SecretReferenceButton field-name="connection string" label="connection string" />
          </span>
        </div>
      </template>

      <OneTimeCodeField v-if="store.selectedItem.hasOneTimePassword" :vault-i-d="store.selectedVaultID" :item-i-d="store.selectedItem.id" />

      <div v-for="link in store.selectedItem.urls" :key="link" class="field" v-bind="bindCopyRow(() => copyText(link, websiteLabel))">
        <div class="field-copy-target" v-bind="bindCopyTarget()">
        <span class="copy-prefix">Copy </span><span class="field-label">{{ websiteLabel }}</span>
        <span class="field-value">{{ link }}</span>
        </div>
        <span class="field-actions">
          <button class="ghost icon-button" aria-label="Open website" title="Open in browser" @click="openWebsite(link)"><AppIcon name="external" /></button>
          <button class="ghost icon-button" :aria-label="`Copy ${websiteLabel.toLowerCase()}`" title="Copy" @click="copyText(link, websiteLabel)"><AppIcon name="copy" /></button>
        </span>
      </div>

      <div
        v-for="androidApp in store.selectedItem.androidApps ?? []"
        :key="`${androidApp.packageName} ${androidApp.certificateSha256}`"
        class="field"
        data-testid="android-app-row"
      >
        <span class="field-label">Android app</span>
        <span class="field-value">
          {{ androidApp.packageName }}
          <span class="android-app-certificate monospace muted">SHA-256 {{ androidApp.certificateSha256 }}</span>
        </span>
      </div>

      <div v-for="field in store.selectedItem.fields" :key="field.name" class="field" v-bind="bindFieldRow(field.name, field.name)">
        <div class="field-copy-target" v-bind="bindCopyTarget()">
        <span class="copy-prefix">Copy </span><span class="field-label">{{ field.name }}</span>
        <span class="field-value" :class="{ monospace: field.concealed }">
          {{ field.concealed && !allRevealedFieldName.includes(field.name) ? maskSecret(field.value) : field.value }}
        </span>
        </div>
        <span class="field-actions">
          <button
            v-if="field.concealed"
            class="ghost icon-button"
            :aria-label="`${allRevealedFieldName.includes(field.name) ? 'Hide' : 'Reveal'} ${field.name}`"
            @click="toggleFieldReveal(field.name)"
          >
            <AppIcon :name="allRevealedFieldName.includes(field.name) ? 'eyeOff' : 'eye'" />
          </button>
          <button class="ghost icon-button" :aria-label="`Copy ${field.name}`" title="Copy" @click="copyField(field.name, field.name)"><AppIcon name="copy" /></button>
          <SecretReferenceButton v-if="field.concealed" :field-name="field.name" :label="field.name" />
        </span>
      </div>

      <p
        v-if="
          !store.selectedItem.username &&
          !store.selectedItem.password &&
          store.selectedItem.urls.length === 0 &&
          store.selectedItem.fields.length === 0 &&
          (store.selectedItem.passkeys ?? []).length === 0 &&
          (store.selectedItem.androidApps ?? []).length === 0 &&
          !server?.host &&
          !server?.port &&
          !server?.sshKeyId &&
          allDatabaseRow.length === 0 &&
          !database?.options
        "
        class="muted no-fields"
      >
        This item has no fields yet.
      </p>
    </section>

    <PasskeyList v-if="(store.selectedItem.passkeys ?? []).length > 0" :passkeys="store.selectedItem.passkeys ?? []" :can-delete="canEdit" />

    <section v-if="store.selectedItem.notes && !isSecureNote" class="card notes-card copy-block" data-field-menu-row v-bind="bindFieldRow('notes', 'Notes')">
      <div class="block-copy-target" v-bind="bindCopyTarget()">
        <span class="copy-prefix">Copy </span><p class="section-label">Notes</p>
        <p class="notes">{{ store.selectedItem.notes }}</p>
      </div>
      <div class="row field-actions" data-field-menu-actions>
        <button class="ghost icon-button" aria-label="Copy notes" title="Copy" @click="copyField('notes', 'Notes')"><AppIcon name="copy" /></button>
      </div>
    </section>

    <p v-if="canEdit && store.selectedItem.canConvertToSecureNote" class="convert-note muted" data-testid="convert-note">
      This login has no username, password or website.
      <button class="ghost" data-testid="convert-to-secure-note" @click="convertToSecureNote"><AppIcon name="note" /> Convert to secure note</button>
    </p>

    <section v-if="allItemShareLink.length > 0" class="card share-links-card" data-testid="item-share-links">
      <p class="section-label">Shared links</p>
      <SharedLinksList :all-share-link="allItemShareLink" :is-title-shown="false" />
    </section>

    <ContextMenu v-if="fieldMenu" :label="fieldMenu.label" :x="fieldMenu.x" :y="fieldMenu.y" :all-action="fieldMenu.allAction" @close="fieldMenu = null" />
    <ContextMenu
      v-if="itemMenu"
      :label="store.selectedItem.title"
      :x="itemMenu.x"
      :y="itemMenu.y"
      :all-action="buildItemMenuActions()"
      @close="closeItemMenu"
    />
    <MoveItemDialog v-if="dialog === 'move'" :vault-i-d="store.selectedVaultID" :item-i-d="store.selectedItem.id" :title="store.selectedItem.title" @close="dialog = null" />
    <ConfirmDeleteDialog v-if="dialog === 'delete'" :vault-i-d="store.selectedVaultID" :item-i-d="store.selectedItem.id" :title="store.selectedItem.title" @close="dialog = null" />
  </article>
</template>

<style scoped>
.item-detail {
  max-width: 640px;
}

header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 22px;
}

.heading {
  flex: 1;
  min-width: 0;
}

h2 {
  margin: 0 0 6px;
  overflow: hidden;
  font-size: var(--font-size-22);
  font-weight: 600;
  letter-spacing: -0.01em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: var(--font-size-12);
}

.badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 7px;
  border: 1px solid var(--color-border-strong);
  border-radius: 10px;
  color: var(--color-text-secondary);
}

.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.tag-chip {
  gap: 4px;
  padding: 1px 8px;
  border-radius: 10px;
  background: var(--color-surface);
  font-size: var(--font-size-12);
  color: var(--color-text-secondary);
}

.tag-chip.selected {
  background: var(--color-selected);
  color: var(--color-text);
}

.header-actions {
  display: flex;
  gap: 4px;
}

.favorite-button.is-favorite {
  color: var(--color-warning);
}

.field-card {
  padding: 4px 0;
}

.field {
  display: grid;
  grid-template-columns: 110px 1fr auto;
  align-items: center;
  gap: 12px;
  min-height: 48px;
  padding: 8px 10px 8px 16px;
  border-bottom: 1px solid var(--color-border);
}

@container detail (max-width: 559px) {
  header {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
  }

  .header-actions {
    grid-column: 1 / -1;
    flex-wrap: wrap;
  }

  .field {
    grid-template-columns: 90px 1fr auto;
  }
}

.field:last-child {
  border-bottom: none;
}

.field-copy-target {
  display: grid;
  grid-column: 1 / span 2;
  grid-template-columns: subgrid;
  align-items: center;
  min-width: 0;
}

.copy-block {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
}

.block-copy-target {
  grid-column: 1 / -1;
  grid-row: 1;
  min-width: 0;
}

.block-copy-target > .field-label,
.block-copy-target > .section-label {
  display: block;
  min-height: 30px;
  padding-right: 180px;
  margin-bottom: 8px;
}

.copy-block > .field-actions {
  grid-column: 2;
  grid-row: 1;
  align-self: start;
}

[data-copy-row] {
  cursor: pointer;
}

.field[data-copy-row]:hover,
.public-key-field[data-copy-row]:hover {
  background: var(--color-hover);
}

[data-copy-target]:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: -2px;
}

.public-key-field {
  padding: 12px 10px 14px 16px;
}

.public-key {
  margin: 8px 0 0;
  padding: 10px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: var(--font-size-12);
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  user-select: text;
  -webkit-user-select: text;
}

.field-label {
  font-size: var(--font-size-12);
  color: var(--color-text-muted);
}

.field-value {
  min-width: 0;
  overflow-wrap: anywhere;
  user-select: text;
  -webkit-user-select: text;
}

.android-app-certificate {
  display: block;
  margin-top: 2px;
  font-size: var(--font-size-11);
}

.secret {
  letter-spacing: 0.06em;
}

.field-actions {
  display: flex;
  gap: 2px;
  opacity: 0.55;
  transition: opacity 120ms;
}

.field:hover .field-actions,
.field:focus-within .field-actions {
  opacity: 1;
}

.no-fields {
  margin: 0;
  padding: 14px 16px;
}

.notes-card {
  margin-top: 14px;
  padding: 14px 16px;
}

.notes {
  margin: 0;
  white-space: pre-wrap;
  user-select: text;
  -webkit-user-select: text;
}

.secure-note {
  margin-top: 0;
}

.notes-card .row {
  margin-bottom: 8px;
}

.notes-card .section-label {
  margin: 0;
}

.note-fields {
  margin-top: 14px;
}

.field-warning {
  display: block;
  margin-top: 2px;
  color: var(--color-warning);
  font-size: var(--font-size-12);
}

.multiline-value {
  white-space: pre-wrap;
}

.convert-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 2px 0;
  font-size: var(--font-size-12);
}

.share-links-card {
  margin-top: 14px;
  padding-top: 14px;
}

.share-links-card .section-label {
  padding: 0 16px;
}
</style>
