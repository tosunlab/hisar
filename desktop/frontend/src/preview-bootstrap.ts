// Development-only: puts the preview UI into a given screen from URL parameters, for screenshots and design review.
import { nextTick } from "vue";
import {
  openImport,
  openSharing,
  selectItem,
  selectVault,
  startCreatingItem,
  startCreatingSSHKey,
  startEditingItem,
  store,
} from "./store";
import { isSettingsSectionID, saveSelectedSectionID } from "./settingsSections";

function waitForUnlocked(): Promise<void> {
  return new Promise((resolve) => {
    const timer = setInterval(() => {
      if (store.status.state === "unlocked" && store.allVault.length > 0 && !store.isBusy) {
        clearInterval(timer);
        resolve();
      }
    }, 20);
  });
}

function waitForElement(selector: string): Promise<HTMLElement> {
  return new Promise((resolve) => {
    const timer = setInterval(() => {
      const element = document.querySelector<HTMLElement>(selector);
      if (element && !store.isBusy) {
        clearInterval(timer);
        resolve(element);
      }
    }, 20);
  });
}

async function clickElement(selector: string): Promise<void> {
  (await waitForElement(selector)).click();
  await nextTick();
}

async function applySettingsParameter(parameter: URLSearchParams): Promise<void> {
  const searchText = parameter.get("settingsSearch");
  if (searchText) {
    const searchInput = await waitForElement('[data-testid="settings-search"]');
    (searchInput as HTMLInputElement).value = searchText;
    searchInput.dispatchEvent(new Event("input"));
    await nextTick();
  }
  if (parameter.get("invite") === "1") {
    await clickElement('[data-testid="start-invite"]');
    const emailInput = await waitForElement('input[name="inviteEmail"]');
    (emailInput as HTMLInputElement).value = "zeynep.demir@example.com";
    emailInput.dispatchEvent(new Event("input"));
    await nextTick();
    (await waitForElement('[data-testid="invite-form"]')).dispatchEvent(new Event("submit"));
  } else if (parameter.get("invite") === "form") {
    await clickElement('[data-testid="start-invite"]');
  }
  const deviceID = parameter.get("revoke");
  if (deviceID) {
    await clickElement(`[data-testid="revoke-device-${deviceID}"]`);
  }
  await waitForElement('[data-testid="people-section"]');
}

// applySharingParameter opens the sharing panel; &code=<account> expands a security code, &remove=<account> asks to remove.
async function applySharingParameter(parameter: URLSearchParams): Promise<void> {
  await waitForUnlocked();
  if (!parameter.get("vault")) {
    await selectVault("family");
  }
  await openSharing();
  const codeAccountID = parameter.get("code");
  if (codeAccountID) {
    await clickElement(`[data-testid="security-code-toggle-${codeAccountID}"]`);
  }
  const removeAccountID = parameter.get("remove");
  if (removeAccountID) {
    await clickElement(`[data-testid="remove-member-${removeAccountID}"]`);
  }
}

// applyImportParameter: &import=choose | preview | saving (stops halfway) | done | deleted, &importVault=<vault> picks the target vault.
async function applyImportParameter(parameter: URLSearchParams): Promise<void> {
  await waitForUnlocked();
  await openImport();
  const importVaultID = parameter.get("importVault");
  if (importVaultID) {
    const vaultSelect = await waitForElement('[data-testid="import-vault"]');
    (vaultSelect as HTMLSelectElement).value = importVaultID;
    vaultSelect.dispatchEvent(new Event("change"));
    await nextTick();
  }
  const step = parameter.get("import");
  if (step === "choose" || step === "1") {
    return;
  }
  await clickElement('[data-testid="choose-import-file"]');
  if (step === "preview") {
    return;
  }
  await clickElement('[data-testid="start-import"]');
  if (step === "saving") {
    return;
  }
  await waitForElement('[data-testid="import-done"]');
  if (step === "deleted") {
    await clickElement('[data-testid="delete-import-file"]');
  }
}

export async function applyPreviewParameter(): Promise<void> {
  const parameter = new URLSearchParams(window.location.search);
  if (parameter.get("secretKey") === "1") {
    store.pendingSecretKey = "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M";
  }
  if (parameter.get("settings") === "1") {
    const sectionID = parameter.get("invite") ? "people" : parameter.get("revoke") ? "devices" : parameter.get("settingsSection");
    if (isSettingsSectionID(sectionID)) {
      saveSelectedSectionID(sectionID);
    }
    store.isSettingsOpen = true;
    applySettingsParameter(parameter);
  }
  if (parameter.get("import")) {
    applyImportParameter(parameter);
  }
  if (parameter.get("newItemMenu") === "1") {
    waitForUnlocked().then(() => clickElement('button[aria-label="New item"]'));
  }
  if (parameter.get("approvalDetails") === "1") {
    clickElement('[data-testid="approval-key"]');
  }
  if (parameter.get("error")) {
    store.errorMessage = parameter.get("error") ?? "";
  }
  if (!parameter.get("vault") && !parameter.get("item") && !parameter.get("editor") && parameter.get("sharing") !== "1") {
    return;
  }
  await waitForUnlocked();
  const vaultID = parameter.get("vault");
  if (vaultID) {
    await selectVault(vaultID);
  }
  const itemID = parameter.get("item");
  if (itemID) {
    await selectItem(itemID);
  }
  if (parameter.get("editor") === "create") {
    startCreatingItem();
  } else if (parameter.get("editor") === "ssh-key") {
    startCreatingSSHKey();
  } else if (parameter.get("editor") === "edit") {
    startEditingItem();
  }
  const search = parameter.get("search");
  if (search) {
    store.searchQuery = search;
  }
  if (parameter.get("sharing") === "1") {
    await applySharingParameter(parameter);
  }
}
