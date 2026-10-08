import { reactive } from "vue";
import {
  api,
  getErrorMessage,
  type ApprovalEvent,
  type ItemDetail,
  type SSHAgentStatus,
  type SSHKeyType,
  type Status,
  type VaultSummary,
} from "./api";

export type EditorMode = "view" | "edit" | "create" | "create-ssh-key";

export const store = reactive({
  status: {
    state: "signed-out",
    server: "",
    email: "",
    name: "",
    biometricAvailable: false,
    biometricEnabled: false,
    biometricReady: false,
  } as Status,
  allVault: [] as VaultSummary[],
  selectedVaultID: "",
  allItem: [] as ItemDetail[],
  selectedItem: null as ItemDetail | null,
  editorMode: "view" as EditorMode,
  searchQuery: "",
  pendingSecretKey: "",
  errorMessage: "",
  noticeMessage: "",
  isBusy: false,
  isSettingsOpen: false,
  sshAgentStatus: null as SSHAgentStatus | null,
  sshAgentErrorMessage: "",
  allPendingApproval: [] as ApprovalEvent[],
});

let noticeTimer: ReturnType<typeof setTimeout> | undefined;

export function showNotice(message: string): void {
  store.noticeMessage = message;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => (store.noticeMessage = ""), 2500);
}

export function clearError(): void {
  store.errorMessage = "";
}

// runAction serializes UI actions and turns failures into a visible error message.
async function runAction<Result>(action: () => Promise<Result>): Promise<Result | undefined> {
  store.isBusy = true;
  store.errorMessage = "";
  try {
    return await action();
  } catch (error) {
    store.errorMessage = getErrorMessage(error);
    if (store.errorMessage === "Hisar is locked") {
      clearAllVaultData();
      store.status = { ...store.status, state: "locked" };
    }
    return undefined;
  } finally {
    store.isBusy = false;
  }
}

function clearAllVaultData(): void {
  store.isSettingsOpen = false;
  store.allVault = [];
  store.selectedVaultID = "";
  store.allItem = [];
  store.selectedItem = null;
  store.editorMode = "view";
  store.searchQuery = "";
}

async function applyStatus(status: Status): Promise<void> {
  store.status = status;
  if (status.state === "unlocked") {
    await loadAllVault();
  } else {
    clearAllVaultData();
  }
}

export async function loadStatus(): Promise<void> {
  await runAction(async () => applyStatus(await api.getStatus()));
}

export async function signIn(server: string, email: string, secretKey: string, password: string): Promise<boolean> {
  const status = await runAction(() => api.signIn(server, email, secretKey, password));
  if (status) {
    await applyStatus(status);
  }
  return status !== undefined;
}

export async function signUp(inviteLink: string, email: string, name: string, password: string): Promise<boolean> {
  const result = await runAction(() => api.signUp(inviteLink, email, name, password));
  if (result) {
    store.pendingSecretKey = result.secretKey;
    await applyStatus(result.status);
  }
  return result !== undefined;
}

export function confirmSecretKeySaved(): void {
  store.pendingSecretKey = "";
}

export async function unlock(password: string): Promise<boolean> {
  const status = await runAction(() => api.unlock(password));
  if (status) {
    await applyStatus(status);
  }
  return status !== undefined;
}

export function isCancelledTouchID(message: string): boolean {
  return /cancel/i.test(message);
}

// unlockWithBiometric does not use runAction: the prompt can wait long, and cancelling is not an error.
export async function unlockWithBiometric(): Promise<boolean> {
  try {
    const status = await api.unlockWithBiometric();
    store.errorMessage = "";
    await applyStatus(status);
    return true;
  } catch (error) {
    const message = getErrorMessage(error);
    if (!isCancelledTouchID(message)) {
      store.errorMessage = message;
    }
    const status = await api.getStatus().catch(() => undefined);
    if (status) {
      store.status = status;
    }
    return false;
  }
}

export async function enableBiometricUnlock(password: string): Promise<boolean> {
  const status = await runAction(() => api.enableBiometricUnlock(password));
  if (status) {
    store.status = status;
    showNotice("Touch ID unlock is on");
  }
  return status !== undefined;
}

export async function disableBiometricUnlock(): Promise<void> {
  const status = await runAction(() => api.disableBiometricUnlock());
  if (status) {
    store.status = status;
    showNotice("Touch ID unlock is off");
  }
}

export async function lock(): Promise<void> {
  const status = await runAction(() => api.lock());
  if (status) {
    await applyStatus(status);
  }
}

export function handleLocked(): void {
  clearAllVaultData();
  store.status = { ...store.status, state: "locked" };
  store.errorMessage = "";
}

export async function signOut(): Promise<void> {
  const status = await runAction(() => api.signOut());
  if (status) {
    store.pendingSecretKey = "";
    await applyStatus(status);
  }
}

export async function loadAllVault(): Promise<void> {
  const allVault = await runAction(() => api.listAllVault());
  if (!allVault) {
    return;
  }
  store.allVault = allVault;
  const stillExists = allVault.some((vault) => vault.id === store.selectedVaultID);
  if (!stillExists && allVault.length > 0) {
    await selectVault(allVault[0].id);
  }
}

export async function selectVault(vaultID: string): Promise<void> {
  store.selectedVaultID = vaultID;
  store.selectedItem = null;
  store.editorMode = "view";
  store.searchQuery = "";
  const allItem = await runAction(() => api.listAllItem(vaultID));
  store.allItem = allItem ? sortAllItem(allItem) : [];
  updateSelectedVaultItemCount();
}

export async function createVault(name: string): Promise<boolean> {
  const vault = await runAction(() => api.createVault(name));
  if (!vault) {
    return false;
  }
  await loadAllVault();
  await selectVault(vault.id);
  return true;
}

export function sortAllItem(allItem: ItemDetail[]): ItemDetail[] {
  return [...allItem].sort((first, second) => first.title.localeCompare(second.title, undefined, { sensitivity: "base" }));
}

export function filterAllItem(allItem: ItemDetail[], query: string): ItemDetail[] {
  const normalizedQuery = query.trim().toLowerCase();
  if (!normalizedQuery) {
    return allItem;
  }
  return allItem.filter((vaultItem) =>
    [vaultItem.title, vaultItem.username, ...vaultItem.urls].some((text) => text.toLowerCase().includes(normalizedQuery)),
  );
}

function updateSelectedVaultItemCount(): void {
  const vault = store.allVault.find((candidate) => candidate.id === store.selectedVaultID);
  if (vault) {
    vault.itemCount = store.allItem.length;
  }
}

export async function selectItem(itemID: string): Promise<void> {
  const detail = await runAction(() => api.getItem(store.selectedVaultID, itemID));
  if (detail) {
    store.selectedItem = detail;
    store.editorMode = "view";
  }
}

export function startCreatingItem(): void {
  store.selectedItem = null;
  store.editorMode = "create";
}

export function startCreatingSSHKey(): void {
  store.selectedItem = null;
  store.editorMode = "create-ssh-key";
}

async function showCreatedItem(created: ItemDetail): Promise<void> {
  const allItem = await runAction(() => api.listAllItem(store.selectedVaultID));
  if (allItem) {
    store.allItem = sortAllItem(allItem);
    updateSelectedVaultItemCount();
  }
  store.selectedItem = created;
  store.editorMode = "view";
}

export async function generateSSHKey(title: string, keyType: SSHKeyType): Promise<boolean> {
  const created = await runAction(() => api.generateSSHKey(store.selectedVaultID, title, keyType));
  if (!created) {
    return false;
  }
  await showCreatedItem(created);
  showNotice("SSH key generated");
  return true;
}

export async function importSSHKey(title: string, privateKeyText: string, passphrase: string): Promise<boolean> {
  const created = await runAction(() => api.importSSHKey(store.selectedVaultID, title, privateKeyText, passphrase));
  if (!created) {
    return false;
  }
  await showCreatedItem(created);
  showNotice("SSH key imported");
  return true;
}

export function startEditingItem(): void {
  if (store.selectedItem) {
    store.editorMode = "edit";
  }
}

export function cancelEditing(): void {
  store.editorMode = "view";
}

export async function saveItem(detail: ItemDetail): Promise<boolean> {
  const saved = await runAction(() => api.saveItem(store.selectedVaultID, detail));
  if (!saved) {
    return false;
  }
  const allItem = await runAction(() => api.listAllItem(store.selectedVaultID));
  if (allItem) {
    store.allItem = sortAllItem(allItem);
    updateSelectedVaultItemCount();
  }
  store.selectedItem = saved;
  store.editorMode = "view";
  showNotice("Saved");
  return true;
}

export async function deleteItem(itemID: string): Promise<void> {
  const deleted = await runAction(async () => {
    await api.deleteItem(store.selectedVaultID, itemID);
    return true;
  });
  if (deleted) {
    store.allItem = store.allItem.filter((vaultItem) => vaultItem.id !== itemID);
    updateSelectedVaultItemCount();
    store.selectedItem = null;
    store.editorMode = "view";
    showNotice("Deleted");
  }
}

export async function copyField(fieldName: string, label: string): Promise<void> {
  if (!store.selectedItem) {
    return;
  }
  const itemID = store.selectedItem.id;
  const copied = await runAction(async () => {
    await api.copyField(store.selectedVaultID, itemID, fieldName);
    return true;
  });
  if (copied) {
    showNotice(`${label} copied, clears in 90 seconds`);
  }
}

// openWebsite only opens http(s) links, so stored items cannot launch other URL schemes.
export async function openWebsite(link: string): Promise<void> {
  const normalizedLink = link.includes("://") ? link : `https://${link}`;
  let protocol = "";
  try {
    protocol = new URL(normalizedLink).protocol;
  } catch {
    protocol = "";
  }
  if (protocol !== "https:" && protocol !== "http:") {
    store.errorMessage = "Only http and https websites can be opened";
    return;
  }
  await runAction(() => api.openURL(normalizedLink));
}

// SSH agent actions report errors inside the settings section, not as a page error.
export async function loadSSHAgentStatus(): Promise<void> {
  try {
    store.sshAgentStatus = await api.getSSHAgentStatus();
    store.sshAgentErrorMessage = "";
  } catch (error) {
    store.sshAgentErrorMessage = getErrorMessage(error);
  }
}

async function changeSSHConfig(change: () => Promise<SSHAgentStatus>, notice: string): Promise<void> {
  store.isBusy = true;
  try {
    const status = await change();
    store.sshAgentStatus = status;
    store.sshAgentErrorMessage = "";
    showNotice(status.backupPath ? `${notice}; backup saved` : notice);
  } catch (error) {
    const message = getErrorMessage(error);
    await loadSSHAgentStatus();
    store.sshAgentErrorMessage = message;
  } finally {
    store.isBusy = false;
  }
}

export async function configureSSHConfig(): Promise<void> {
  await changeSSHConfig(() => api.configureSSHConfig(), "SSH now uses Hisar");
}

export async function removeSSHConfig(): Promise<void> {
  await changeSSHConfig(() => api.removeSSHConfig(), "Removed Hisar from the SSH config");
}

export function addSSHApproval(approval: ApprovalEvent): void {
  if (!store.allPendingApproval.some((pending) => pending.requestID === approval.requestID)) {
    store.allPendingApproval.push(approval);
  }
}

export function removeSSHApproval(requestID: string): void {
  store.allPendingApproval = store.allPendingApproval.filter((pending) => pending.requestID !== requestID);
}

export async function respondSSHApproval(requestID: string, isAllowed: boolean): Promise<void> {
  removeSSHApproval(requestID);
  await api.respondSSHApproval(requestID, isAllowed).catch(() => undefined);
}

export async function copyText(text: string, label: string): Promise<void> {
  const copied = await runAction(async () => {
    await api.copyText(text);
    return true;
  });
  if (copied) {
    showNotice(`${label} copied, clears in 90 seconds`);
  }
}
