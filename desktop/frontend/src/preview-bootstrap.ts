// Development-only: puts the preview UI into a given screen from URL parameters, for screenshots and design review.
import { nextTick } from "vue";
import {
  openItemShareLinkDialog,
  openShareLinks,
  openTextShareLinkDialog,
  openImport,
  openSharing,
  openVaultDialog,
  selectFavoriteItem,
  selectFavorites,
  selectItem,
  selectVault,
  startCreatingItem,
  startCreatingCard,
  startCreatingIdentity,
  startCreatingAPICredential,
  startCreatingAddress,
  startCreatingSoftwareLicense,
  startCreatingServer,
  startCreatingDatabase,
  startCreatingSecureNote,
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

async function typeIntoInput(selector: string, text: string): Promise<void> {
  const input = (await waitForElement(selector)) as HTMLInputElement;
  input.value = text;
  input.dispatchEvent(new Event("input"));
  await nextTick();
}

// applyAccountDialogParameter: &accountDialog=change-master-password | replace-secret-key | replace-secret-key-done | delete-account | delete-account-filled.
async function applyAccountDialogParameter(dialog: string | null): Promise<void> {
  if (dialog === "change-master-password") {
    await clickElement('[data-testid="change-master-password"]');
    await typeIntoInput('input[name="currentPassword"]', "current master password");
    await typeIntoInput('input[name="newPassword"]', "orange cactus harbor lantern");
    await typeIntoInput('input[name="repeatedPassword"]', "orange cactus harbor lantern");
  } else if (dialog === "replace-secret-key" || dialog === "replace-secret-key-done") {
    await clickElement('[data-testid="replace-secret-key"]');
    if (dialog === "replace-secret-key-done") {
      await typeIntoInput('input[name="replaceSecretKeyPassword"]', "current master password");
      await clickElement('[data-testid="confirm-replace-secret-key"]');
      await waitForElement('[data-testid="new-secret-key"]');
    }
  } else if (dialog === "delete-account" || dialog === "delete-account-filled") {
    await clickElement('[data-testid="delete-account"]');
    await waitForElement('[data-testid="delete-account-consequences"]');
    if (dialog === "delete-account-filled") {
      await typeIntoInput('input[name="deleteAccountPassword"]', "current master password");
      await typeIntoInput('input[name="deleteAccountEmail"]', "alper@tryhisar.com");
    }
  }
}

async function applySettingsParameter(parameter: URLSearchParams): Promise<void> {
  const searchText = parameter.get("settingsSearch");
  if (searchText) {
    const searchInput = await waitForElement('[data-testid="settings-search"]');
    (searchInput as HTMLInputElement).value = searchText;
    searchInput.dispatchEvent(new Event("input"));
    await nextTick();
  }
  await applyAccountDialogParameter(parameter.get("accountDialog"));
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
  if (parameter.get("exportFormat") === "plain") {
    await clickElement('[data-testid="export-format-plain"]');
  } else if (parameter.get("exportPassphrase") === "1") {
    for (const selector of ['[data-testid="export-passphrase"]', '[data-testid="export-passphrase-repeat"]']) {
      const passphraseInput = (await waitForElement(selector)) as HTMLInputElement;
      passphraseInput.value = "orange cactus harbor";
      passphraseInput.dispatchEvent(new Event("input"));
    }
    await nextTick();
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
    await selectVault("team");
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

// applyShareLinkParameter: &shareLink=dialog | passphrase | created | text (with &item=), or &sharedLinks=1 for the overview.
async function applyShareLinkParameter(parameter: URLSearchParams): Promise<void> {
  const step = parameter.get("shareLink");
  if (step === "text") {
    openTextShareLinkDialog();
    return;
  }
  await openItemShareLinkDialog();
  if (step === "passphrase" || step === "created") {
    await clickElement('[data-testid="share-passphrase-required"]');
    const passphraseInput = await waitForElement('[data-testid="share-passphrase"]');
    (passphraseInput as HTMLInputElement).value = "orange-cactus-harbor";
    passphraseInput.dispatchEvent(new Event("input"));
    await nextTick();
  }
  if (step === "created") {
    await clickElement('[data-testid="create-share-link"]');
    await waitForElement('[data-testid="share-link-created"]');
  }
}

export async function applyPreviewParameter(): Promise<void> {
  const parameter = new URLSearchParams(window.location.search);
  if (parameter.get("secretKey") === "1") {
    store.pendingSecretKey = "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M";
  }
  if (parameter.get("touchIdOffer") === "1") {
    store.isTouchIDOfferShown = true;
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
  const vaultDialogMode = parameter.get("vaultDialog");
  if (vaultDialogMode === "rename" || vaultDialogMode === "delete" || vaultDialogMode === "leave") {
    waitForUnlocked().then(async () => {
      await openVaultDialog(parameter.get("dialogVault") ?? "team", vaultDialogMode);
      if (parameter.get("deleteItems") === "1") {
        await clickElement('[data-testid="vault-delete-items"]');
      }
    });
  }
  if (parameter.get("vaultMenu") === "1") {
    waitForUnlocked().then(() => clickElement('[data-testid="vault-actions"]'));
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
  if (parameter.get("favorites") === "1") {
    await waitForUnlocked();
    await waitForElement('[data-testid="open-favorites"]');
    while (!store.selectedVaultID || store.isBusy) {
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
    await selectFavorites();
    const favorite = store.allFavorite.find((candidate) => candidate.item.id === parameter.get("item"));
    if (favorite) {
      await selectFavoriteItem(favorite);
    }
    return;
  }
  if (!parameter.get("vault") && !parameter.get("item") && !parameter.get("editor") && parameter.get("sharing") !== "1" && !parameter.get("shareLink") && parameter.get("sharedLinks") !== "1") {
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
    if (parameter.get("commitSigning") === "1") {
      await clickElement('[data-testid="set-up-commit-signing"]');
    }
  }
  if (parameter.get("editor") === "create") {
    startCreatingItem();
  } else if (parameter.get("editor") === "secure-note") {
    startCreatingSecureNote();
  } else if (parameter.get("editor") === "credit-card") {
    startCreatingCard();
  } else if (parameter.get("editor") === "identity") {
    startCreatingIdentity();
  } else if (parameter.get("editor") === "api-credential") {
    startCreatingAPICredential();
  } else if (parameter.get("editor") === "address") {
    startCreatingAddress();
  } else if (parameter.get("editor") === "software-license") {
    startCreatingSoftwareLicense();
  } else if (parameter.get("editor") === "server") {
    startCreatingServer();
  } else if (parameter.get("editor") === "database") {
    startCreatingDatabase();
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
  if (parameter.get("shareLink")) {
    await applyShareLinkParameter(parameter);
  }
  if (parameter.get("sharedLinks") === "1") {
    await openShareLinks();
  }
}
