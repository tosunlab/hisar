import { reactive } from "vue";
import {
  api,
  getErrorMessage,
  type BrowserPairingRequest,
  type BrowserStatus,
  type Device,
  type FamilyMember,
  type FamilyRole,
  type ImportFormat,
  type ImportPreview,
  type ImportProgress,
  type ItemDetail,
  type PendingRecovery,
  type RecoveryStatus,
  type SSHAgentStatus,
  type SSHKeyType,
  type Status,
  type UpdateStatus,
  type VaultMember,
  type VaultRole,
  type VaultSummary,
} from "./api";

export type EditorMode = "view" | "edit" | "create" | "create-ssh-key";
export type ImportStep = "choose" | "preview" | "saving" | "done";

export const store = reactive({
  status: {
    state: "signed-out",
    server: "",
    email: "",
    name: "",
    biometricAvailable: false,
    biometricEnabled: false,
    biometricReady: false,
    isOffline: false,
  } as Status,
  allVault: [] as VaultSummary[],
  selectedVaultID: "",
  allItem: [] as ItemDetail[],
  selectedItem: null as ItemDetail | null,
  editorMode: "view" as EditorMode,
  searchQuery: "",
  pendingSecretKey: "",
  isRecoverySubmitted: false,
  errorMessage: "",
  noticeMessage: "",
  isBusy: false,
  isSettingsOpen: false,
  sshAgentStatus: null as SSHAgentStatus | null,
  sshAgentErrorMessage: "",
  isSharingOpen: false,
  allVaultMember: [] as VaultMember[],
  allFamilyMember: [] as FamilyMember[],
  allDevice: [] as Device[],
  browserPairing: null as BrowserPairingRequest | null,
  browserStatus: null as BrowserStatus | null,
  updateStatus: null as UpdateStatus | null,
  recoveryStatus: null as RecoveryStatus | null,
  allPendingRecovery: [] as PendingRecovery[],
  isVaultRecoveryOpen: false,
  isImportOpen: false,
  importStep: "choose" as ImportStep,
  allImportFormat: [] as ImportFormat[],
  importPreview: null as ImportPreview | null,
  importProgress: null as ImportProgress | null,
  isImportFileDeleted: false,
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
  resetImport();
  store.isSettingsOpen = false;
  store.isSharingOpen = false;
  store.allVaultMember = [];
  store.allFamilyMember = [];
  store.allDevice = [];
  store.recoveryStatus = null;
  store.allPendingRecovery = [];
  store.isVaultRecoveryOpen = false;
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
    store.isRecoverySubmitted = false;
    await applyStatus(status);
  }
  return status !== undefined;
}

export async function signUp(inviteLink: string, email: string, name: string, password: string): Promise<boolean> {
  const result = await runAction(() => api.signUp(inviteLink, email, name, password));
  if (result) {
    store.pendingSecretKey = result.secretKey;
    store.isRecoverySubmitted = false;
    await applyStatus(result.status);
  }
  return result !== undefined;
}

export function confirmSecretKeySaved(): void {
  store.pendingSecretKey = "";
  api.forgetNewAccountSecretKey().catch(() => undefined);
}

// recoverAccount shows the new Secret Key; the account can sign in with it once an admin completes the recovery.
export async function recoverAccount(recoveryLink: string, email: string, newPassword: string): Promise<boolean> {
  const result = await runAction(() => api.recoverAccount(recoveryLink, email, newPassword));
  if (result) {
    store.pendingSecretKey = result.secretKey;
    store.isRecoverySubmitted = true;
    await applyStatus(result.status);
  }
  return result !== undefined;
}

function showSavedNotice(savedPath: string | undefined): boolean {
  if (!savedPath) {
    return false;
  }
  showNotice(`Emergency Kit saved to ${savedPath.split("/").pop()}`);
  return true;
}

export async function saveNewAccountEmergencyKit(): Promise<boolean> {
  return showSavedNotice(await runAction(() => api.saveNewAccountEmergencyKit()));
}

export async function saveEmergencyKit(password: string): Promise<boolean> {
  return showSavedNotice(await runAction(() => api.saveEmergencyKit(password)));
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

export const syncInterval = 60_000;

// syncInBackground refreshes vaults and the open item without showing errors; the offline notice covers a lost connection.
export async function syncInBackground(): Promise<void> {
  if (store.status.state !== "unlocked" || store.isBusy || store.editorMode !== "view") {
    return;
  }
  try {
    const allVault = await api.listAllVault();
    const status = await api.getStatus();
    if (store.status.state !== "unlocked" || status.state !== "unlocked") {
      return;
    }
    store.allVault = allVault;
    store.status = status;
    if (!allVault.some((vault) => vault.id === store.selectedVaultID)) {
      if (allVault.length > 0) {
        await selectVault(allVault[0].id);
      }
      return;
    }
    const allItem = sortAllItem(await api.listAllItem(store.selectedVaultID));
    if (store.editorMode !== "view") {
      return;
    }
    store.allItem = allItem;
    const selectedItemID = store.selectedItem?.id;
    if (selectedItemID) {
      store.selectedItem = allItem.some((vaultItem) => vaultItem.id === selectedItemID) ? await api.getItem(store.selectedVaultID, selectedItemID) : null;
    }
  } catch {
    return;
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

export async function deletePasskey(credentialID: string): Promise<void> {
  if (!store.selectedItem) {
    return;
  }
  const itemID = store.selectedItem.id;
  const updated = await runAction(() => api.deletePasskey(store.selectedVaultID, itemID, credentialID));
  if (updated) {
    store.allItem = store.allItem.map((vaultItem) => (vaultItem.id === itemID ? { ...vaultItem, passkeys: updated.passkeys } : vaultItem));
    if (store.selectedItem?.id === itemID) {
      store.selectedItem = updated;
    }
    showNotice("Passkey deleted");
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

export function getSelectedVault(): VaultSummary | undefined {
  return store.allVault.find((vault) => vault.id === store.selectedVaultID);
}

export function getFirstName(member: { name: string; email: string }): string {
  return member.name.trim().split(/\s+/)[0] || member.email;
}

export async function loadSharing(): Promise<void> {
  const vaultID = store.selectedVaultID;
  const result = await runAction(() => Promise.all([api.listAllVaultMember(vaultID), api.listAllMember()]));
  if (result && vaultID === store.selectedVaultID) {
    [store.allVaultMember, store.allFamilyMember] = result;
  }
}

export async function openSharing(): Promise<void> {
  store.allVaultMember = [];
  store.isSharingOpen = true;
  await loadSharing();
}

export function closeSharing(): void {
  store.isSharingOpen = false;
  store.errorMessage = "";
}

export async function shareVault(member: FamilyMember, role: VaultRole): Promise<boolean> {
  const vaultID = store.selectedVaultID;
  const shared = await runAction(async () => {
    await api.shareVault(vaultID, member.email, role);
    return true;
  });
  if (shared) {
    await loadSharing();
    showNotice(`Shared with ${getFirstName(member)}`);
  }
  return shared === true;
}

export async function changeVaultMemberRole(member: VaultMember, role: VaultRole): Promise<boolean> {
  const vaultID = store.selectedVaultID;
  const changed = await runAction(async () => {
    await api.changeVaultMemberRole(vaultID, member.accountId, role);
    return true;
  });
  await loadSharing();
  if (changed) {
    showNotice(`${getFirstName(member)} is now ${role === "owner" ? "an owner" : `a ${role}`}`);
  }
  return changed === true;
}

// removeVaultMember also reloads the items: the vault was re-encrypted, so every item has a new revision.
export async function removeVaultMember(member: VaultMember): Promise<boolean> {
  const vaultID = store.selectedVaultID;
  const removed = await runAction(async () => {
    await api.removeVaultMember(vaultID, member.accountId);
    return true;
  });
  if (!removed) {
    return false;
  }
  await loadSharing();
  const allItem = await runAction(() => api.listAllItem(vaultID));
  if (allItem && vaultID === store.selectedVaultID) {
    store.allItem = sortAllItem(allItem);
    store.selectedItem = null;
    store.editorMode = "view";
  }
  showNotice(`Removed ${getFirstName(member)} and re-encrypted the vault`);
  return true;
}

export async function loadSettingsData(): Promise<void> {
  const result = await runAction(() => Promise.all([api.listAllMember(), api.listAllDevice()]));
  if (result) {
    [store.allFamilyMember, store.allDevice] = result;
  }
}

export async function inviteMember(email: string, role: FamilyRole): Promise<string | undefined> {
  const link = await runAction(() => api.inviteMember(email, role));
  if (link) {
    showNotice("Invite link created");
  }
  return link;
}

export async function revokeDevice(device: Device): Promise<boolean> {
  const revoked = await runAction(async () => {
    await api.revokeDevice(device.id);
    return true;
  });
  if (!revoked) {
    return false;
  }
  const allDevice = await runAction(() => api.listAllDevice());
  if (allDevice) {
    store.allDevice = allDevice;
  }
  showNotice(`${device.name} was signed out`);
  return true;
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

export function showBrowserPairing(request: BrowserPairingRequest | null): void {
  store.browserPairing = request;
  loadBrowserStatus();
}

export async function cancelBrowserPairing(): Promise<void> {
  store.browserPairing = null;
  await api.cancelBrowserPairing().catch(() => undefined);
}

export async function loadBrowserStatus(): Promise<void> {
  try {
    store.browserStatus = await api.getBrowserStatus();
  } catch {
    store.browserStatus = null;
  }
}

// restoreBrowserPairing shows a pairing that started before the window loaded; later ones arrive as events.
export async function restoreBrowserPairing(): Promise<void> {
  await loadBrowserStatus();
  store.browserPairing = store.browserStatus?.pendingPairing ?? null;
}

export async function removePairedBrowser(instanceID: string): Promise<boolean> {
  const browserStatus = await runAction(() => api.removePairedBrowser(instanceID));
  if (!browserStatus) {
    return false;
  }
  store.browserStatus = browserStatus;
  showNotice("Browser removed; it must be paired again to use Hisar");
  return true;
}

export async function setExactHostMatch(isExactHostMatch: boolean): Promise<void> {
  const browserStatus = await runAction(() => api.setExactHostMatch(isExactHostMatch));
  if (browserStatus) {
    store.browserStatus = browserStatus;
  }
}

export async function loadUpdateStatus(): Promise<void> {
  try {
    store.updateStatus = await api.getUpdateStatus();
  } catch {
    store.updateStatus = null;
  }
}

export async function checkForUpdates(): Promise<void> {
  const updateStatus = await runAction(() => api.checkForUpdates());
  if (updateStatus) {
    store.updateStatus = updateStatus;
    showNotice("Checking for updates…");
  }
}

export async function setAutomaticUpdateCheck(isEnabled: boolean): Promise<void> {
  const updateStatus = await runAction(() => api.setAutomaticUpdateCheck(isEnabled));
  if (updateStatus) {
    store.updateStatus = updateStatus;
  }
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

export async function loadRecoveryData(): Promise<void> {
  const recoveryStatus = await runAction(() => api.getRecoveryStatus());
  if (!recoveryStatus) {
    return;
  }
  store.recoveryStatus = recoveryStatus;
  if (recoveryStatus.isAdmin && recoveryStatus.isSetUp) {
    const allPendingRecovery = await runAction(() => api.listAllPendingRecovery());
    if (allPendingRecovery) {
      store.allPendingRecovery = allPendingRecovery;
    }
  }
}

async function reloadVaultRecovery(): Promise<void> {
  const allVault = await runAction(() => api.listAllVault());
  if (allVault) {
    store.allVault = allVault;
  }
}

// setUpRecovery returns the vaults that could not be made recoverable, or undefined when it failed.
export async function setUpRecovery(): Promise<string[] | undefined> {
  const allFailedVaultName = await runAction(() => api.setUpRecovery());
  if (!allFailedVaultName) {
    return undefined;
  }
  await loadRecoveryData();
  await reloadVaultRecovery();
  showNotice("Account recovery is set up");
  return allFailedVaultName;
}

export async function enableRecoveryForAllVault(): Promise<string[] | undefined> {
  const allFailedVaultName = await runAction(() => api.enableRecoveryForAllVault());
  if (!allFailedVaultName) {
    return undefined;
  }
  await reloadVaultRecovery();
  showNotice("Your vaults can be recovered");
  return allFailedVaultName;
}

export async function setVaultRecovery(vaultID: string, isEnabled: boolean): Promise<boolean> {
  const summary = await runAction(() => (isEnabled ? api.enableVaultRecovery(vaultID) : api.disableVaultRecovery(vaultID)));
  if (!summary) {
    return false;
  }
  const vault = store.allVault.find((candidate) => candidate.id === vaultID);
  if (vault) {
    vault.recoverable = summary.recoverable;
    vault.recoveryOptOut = summary.recoveryOptOut;
  }
  showNotice(isEnabled ? `${summary.name} can be recovered again` : `Recovery is off for ${summary.name}`);
  return true;
}

export async function startAccountRecovery(member: FamilyMember): Promise<string | undefined> {
  const link = await runAction(() => api.startAccountRecovery(member.accountId));
  if (link) {
    showNotice("Recovery link created");
  }
  return link;
}

// completeAccountRecovery returns the vaults the member lost, or undefined when it failed.
export async function completeAccountRecovery(pending: PendingRecovery): Promise<string[] | undefined> {
  const allUnrecoverableVaultName = await runAction(() => api.completeAccountRecovery(pending.accountId));
  if (!allUnrecoverableVaultName) {
    return undefined;
  }
  await loadRecoveryData();
  const allFamilyMember = await runAction(() => api.listAllMember());
  if (allFamilyMember) {
    store.allFamilyMember = allFamilyMember;
  }
  showNotice(`Recovered ${getFirstName(pending)}`);
  return allUnrecoverableVaultName;
}

export function openVaultRecovery(): void {
  store.isVaultRecoveryOpen = true;
  loadRecoveryData();
}

export function closeVaultRecovery(): void {
  store.isVaultRecoveryOpen = false;
  store.errorMessage = "";
}

function resetImport(): void {
  store.isImportOpen = false;
  store.importStep = "choose";
  store.importPreview = null;
  store.importProgress = null;
  store.isImportFileDeleted = false;
}

export async function openImport(): Promise<void> {
  resetImport();
  store.isSettingsOpen = false;
  store.isSharingOpen = false;
  store.isVaultRecoveryOpen = false;
  store.errorMessage = "";
  store.isImportOpen = true;
  if (store.allImportFormat.length === 0) {
    store.allImportFormat = (await runAction(() => api.listAllImportFormat())) ?? [];
  }
}

// closeImport drops the parsed export in the backend; it is not offered while items are being saved.
export function closeImport(): void {
  if (store.importStep === "saving") {
    return;
  }
  if (store.importPreview || store.importProgress) {
    api.cancelImport().catch(() => undefined);
  }
  resetImport();
  store.errorMessage = "";
}

export function getAllEditableVault(): VaultSummary[] {
  return store.allVault.filter((vault) => vault.role !== "reader");
}

export async function chooseImportFile(vaultID: string, format: string): Promise<boolean> {
  const preview = await runAction(() => api.chooseImportFile(vaultID, format));
  if (!preview?.fileName) {
    return false;
  }
  store.importPreview = preview;
  store.importStep = "preview";
  return true;
}

export async function changeImportVault(vaultID: string): Promise<void> {
  const preview = await runAction(() => api.previewImport(vaultID));
  if (preview) {
    store.importPreview = preview;
  }
}

export async function startImport(isSkippingDuplicate: boolean): Promise<void> {
  if (!store.importPreview) {
    return;
  }
  const vaultID = store.importPreview.vaultId;
  store.importStep = "saving";
  const progress = await runAction(() => api.startImport(vaultID, isSkippingDuplicate));
  if (!progress) {
    store.importStep = "preview";
    return;
  }
  store.importProgress = progress;
  await continueImport();
}

// continueImport saves the queued items batch by batch; after an error the user can continue where it stopped.
export async function continueImport(): Promise<void> {
  while (store.isImportOpen && store.importProgress && !store.importProgress.isDone) {
    const progress = await runAction(() => api.continueImport());
    if (!progress) {
      return;
    }
    store.importProgress = progress;
  }
  if (store.isImportOpen && store.importProgress?.isDone) {
    await finishImport(store.importProgress);
  }
}

async function finishImport(progress: ImportProgress): Promise<void> {
  store.importStep = "done";
  const allVault = await runAction(() => api.listAllVault());
  if (allVault) {
    store.allVault = allVault;
  }
  if (progress.vaultId === store.selectedVaultID) {
    const allItem = await runAction(() => api.listAllItem(progress.vaultId));
    if (allItem) {
      store.allItem = sortAllItem(allItem);
    }
  }
  showNotice(`Imported ${progress.savedCount} ${progress.savedCount === 1 ? "item" : "items"} into ${progress.vaultName}`);
}

export async function deleteImportFile(): Promise<boolean> {
  const fileName = store.importProgress?.fileName ?? "the export file";
  const deleted = await runAction(async () => {
    await api.deleteImportFile();
    return true;
  });
  if (deleted) {
    store.isImportFileDeleted = true;
    showNotice(`Deleted ${fileName}`);
  }
  return deleted === true;
}
