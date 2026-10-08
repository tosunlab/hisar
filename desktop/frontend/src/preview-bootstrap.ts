// Development-only: puts the preview UI into a given screen from URL parameters, for screenshots and design review.
import { addSSHApproval, selectItem, selectVault, startCreatingItem, startCreatingSSHKey, startEditingItem, store } from "./store";

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

export async function applyPreviewParameter(): Promise<void> {
  const parameter = new URLSearchParams(window.location.search);
  if (parameter.get("secretKey") === "1") {
    store.pendingSecretKey = "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M";
  }
  if (parameter.get("settings") === "1") {
    store.isSettingsOpen = true;
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
  if (!parameter.get("vault") && !parameter.get("item") && !parameter.get("editor")) {
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
}
