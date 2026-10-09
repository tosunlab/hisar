// Development-only: puts the preview UI into a given screen from URL parameters, for screenshots and design review.
import { nextTick } from "vue";
import {
  addSSHApproval,
  openSharing,
  openVaultRecovery,
  selectItem,
  selectVault,
  startCreatingItem,
  startCreatingSSHKey,
  startEditingItem,
  store,
} from "./store";

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

function setInputValue(element: HTMLElement, value: string): void {
  (element as HTMLInputElement).value = value;
  element.dispatchEvent(new Event("input"));
}

// applyRecoveryParameter: &setUpRecovery=1 (setup confirmation), &startRecovery=<account> (created link),
// &completeRecovery=<account> (confirmation), &completedRecovery=<account> (result with the lost vaults).
async function applyRecoveryParameter(parameter: URLSearchParams): Promise<void> {
  if (parameter.get("setUpRecovery") === "1") {
    await clickElement('[data-testid="start-recovery-setup"]');
  }
  const startAccountID = parameter.get("startRecovery");
  if (startAccountID) {
    await clickElement(`[data-testid="start-recovery-${startAccountID}"]`);
    await clickElement('[data-testid="confirm-start-recovery"]');
  }
  const completeAccountID = parameter.get("completeRecovery") ?? parameter.get("completedRecovery");
  if (completeAccountID) {
    await clickElement(`[data-testid="complete-recovery-${completeAccountID}"]`);
    if (parameter.get("completedRecovery")) {
      await clickElement('[data-testid="confirm-complete-recovery"]');
    }
  }
}

async function applySettingsParameter(parameter: URLSearchParams): Promise<void> {
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
  await waitForElement('[data-testid="recovery-state"]');
  await applyRecoveryParameter(parameter);
  const scrollTarget = parameter.get("scroll");
  if (scrollTarget) {
    (await waitForElement(`[data-testid="${scrollTarget}-section"]`)).scrollIntoView();
  }
}

// applyRecoverAccountParameter opens the sign-in screen's recovery form; &recover=filled also fills it in.
async function applyRecoverAccountParameter(parameter: URLSearchParams): Promise<void> {
  await clickElement('[data-testid="open-recover-account"]');
  if (parameter.get("recover") !== "filled") {
    return;
  }
  const formSelector = '[data-testid="recover-account-form"]';
  setInputValue(await waitForElement(`${formSelector} input[name="recoveryLink"]`), "https://tryhisar.com/recover/r7Kp2xQ9mVt4Lw8Nc3Bz6Hd");
  setInputValue(await waitForElement(`${formSelector} input[name="email"]`), "deniz@tryhisar.com");
  setInputValue(await waitForElement(`${formSelector} input[name="password"]`), "kale-kapı-lantern-mosaic-71");
  setInputValue(await waitForElement(`${formSelector} input[name="repeatedPassword"]`), "kale-kapı-lantern-mosaic-71");
  await nextTick();
}

// applyVaultRecoveryParameter opens the personal vault's recovery panel; &disableRecovery=1 shows the warning.
async function applyVaultRecoveryParameter(parameter: URLSearchParams): Promise<void> {
  if (!parameter.get("vault")) {
    await selectVault("personal");
  }
  openVaultRecovery();
  if (parameter.get("disableRecovery") === "1") {
    await clickElement('[data-testid="disable-vault-recovery"]');
  }
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

export async function applyPreviewParameter(): Promise<void> {
  const parameter = new URLSearchParams(window.location.search);
  if (parameter.get("secretKey") === "1" || parameter.get("secretKey") === "recovery") {
    store.pendingSecretKey = "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M";
    store.isRecoverySubmitted = parameter.get("secretKey") === "recovery";
  }
  if (parameter.get("recoverySubmitted") === "1") {
    store.isRecoverySubmitted = true;
  }
  if (parameter.get("recover")) {
    applyRecoverAccountParameter(parameter);
  }
  if (parameter.get("settings") === "1") {
    store.isSettingsOpen = true;
    applySettingsParameter(parameter);
  }
  if (parameter.get("approval") === "1") {
    addSSHApproval({
      requestID: "preview-request",
      clientName: "git",
      clientPath: "/usr/bin/git",
      keyTitle: "GitHub SSH key",
      fingerprint: "SHA256:Jx3o1VZ0kq7vZr6S8m2E3yQ2Lw7bQb4pN5uD9cF1aKs",
      destinationHost: "github.com",
      destinationFingerprint: "SHA256:+DiY3wvvV6TuJJhbpZisF/zLDA0zPMSvHdkr4UvCOqU",
      isForwarded: false,
    });
  }
  if (parameter.get("error")) {
    store.errorMessage = parameter.get("error") ?? "";
  }
  if (!parameter.get("vault") && !parameter.get("item") && !parameter.get("editor") && parameter.get("sharing") !== "1" && parameter.get("vaultRecovery") !== "1") {
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
  if (parameter.get("vaultRecovery") === "1") {
    await applyVaultRecoveryParameter(parameter);
  }
}
