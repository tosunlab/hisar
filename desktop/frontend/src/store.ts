import { reactive } from "vue";
import {
  api,
  getErrorMessage,
  type BrowserPairingRequest,
  type BrowserStatus,
  type CommandLineStatus,
  type Device,
  type Person,
  type ReceivedShareInvitation,
  type SentShareInvitation,
  type ShareAwaitingUnlock,
  type ShareInvitations,
  type ShareLink,
  type ShareLinkField,
  type ShareLinkRequest,
  type ImportFormat,
  type ImportPreview,
  type ImportProgress,
  type ItemDetail,
  type SSHAgentStatus,
  type SSHKeyType,
  type Status,
  type UpdateStatus,
  type VaultMember,
  type VaultRole,
  type VaultSummary,
} from "./api";
import { getPlanLimit, pricingURL, type PlanLimit } from "./plan";
import type { Plan } from "../bindings/github.com/tryhisar/hisar/desktop/vault";

export type EditorMode = "view" | "edit" | "create" | "create-ssh-key";
export type ImportStep = "choose" | "preview" | "saving" | "done";

export interface ShareLinkDialog {
  mode: "item" | "text";
  vaultID: string;
  itemID: string;
  title: string;
  allField: ShareLinkField[];
}

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
    autoLockSeconds: 600,
  } as Status,
  allVault: [] as VaultSummary[],
  selectedVaultID: "",
  allItem: [] as ItemDetail[],
  selectedItem: null as ItemDetail | null,
  editorMode: "view" as EditorMode,
  searchQuery: "",
  pendingSecretKey: "",
  isTouchIDOfferShown: false,
  errorMessage: "",
  noticeMessage: "",
  isBusy: false,
  isSettingsOpen: false,
  sshAgentStatus: null as SSHAgentStatus | null,
  sshAgentErrorMessage: "",
  isSharingOpen: false,
  allVaultMember: [] as VaultMember[],
  allPerson: [] as Person[],
  shareInvitations: { received: [], sent: [], completionError: "" } as ShareInvitations,
  allShareAwaitingUnlock: [] as ShareAwaitingUnlock[],
  allDevice: [] as Device[],
  browserPairing: null as BrowserPairingRequest | null,
  browserStatus: null as BrowserStatus | null,
  updateStatus: null as UpdateStatus | null,
  commandLineStatus: null as CommandLineStatus | null,
  commandLineErrorMessage: "",
  allAgentVaultID: [] as string[],
  agentErrorMessage: "",
  isImportOpen: false,
  importStep: "choose" as ImportStep,
  allImportFormat: [] as ImportFormat[],
  importPreview: null as ImportPreview | null,
  importProgress: null as ImportProgress | null,
  isImportFileDeleted: false,
  allShareLink: [] as ShareLink[],
  isShareLinksOpen: false,
  shareLinkDialog: null as ShareLinkDialog | null,
  plan: null as Plan | null,
  isPlanUnavailable: false,
  planLimit: null as PlanLimit | null,
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
    store.planLimit = getPlanLimit(error);
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
  store.plan = null;
  store.planLimit = null;
  resetImport();
  store.isTouchIDOfferShown = false;
  store.isSettingsOpen = false;
  store.isSharingOpen = false;
  store.allVaultMember = [];
  store.allPerson = [];
  store.shareInvitations = { received: [], sent: [], completionError: "" };
  store.allShareAwaitingUnlock = [];
  store.allDevice = [];
  store.allShareLink = [];
  store.isShareLinksOpen = false;
  store.shareLinkDialog = null;
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

export async function signUp(invite: string, email: string, name: string, password: string): Promise<boolean> {
  const result = await runAction(() => api.signUp(invite, email, name, password));
  if (result) {
    store.pendingSecretKey = result.secretKey;
    await applyStatus(result.status);
  }
  return result !== undefined;
}

export function confirmSecretKeySaved(): void {
  store.pendingSecretKey = "";
  if (store.status.biometricAvailable && !store.status.biometricEnabled) {
    store.isTouchIDOfferShown = true;
    return;
  }
  api.forgetNewAccountSecretKey().catch(() => undefined);
}

function finishTouchIDOffer(): void {
  store.isTouchIDOfferShown = false;
  store.errorMessage = "";
  api.forgetNewAccountSecretKey().catch(() => undefined);
}

// acceptTouchIDOffer does not use runAction: cancelling the Touch ID prompt keeps the offer open without an error.
export async function acceptTouchIDOffer(): Promise<boolean> {
  store.isBusy = true;
  try {
    store.status = await api.enableNewAccountBiometricUnlock();
    finishTouchIDOffer();
    showNotice("Touch ID unlock is on");
    return true;
  } catch (error) {
    const message = getErrorMessage(error);
    store.errorMessage = isCancelledTouchID(message) ? "" : message;
    return false;
  } finally {
    store.isBusy = false;
  }
}

export function declineTouchIDOffer(): void {
  finishTouchIDOffer();
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
  await loadShareInvitationsQuietly();
  await loadAllShareLinkQuietly();
}

// loadShareInvitationsQuietly also lets the backend complete shares people accepted; failures stay silent like a background sync.
export async function loadShareInvitationsQuietly(): Promise<void> {
  try {
    const shareInvitations = await api.listAllShareInvitation();
    if (store.status.state === "unlocked") {
      store.shareInvitations = shareInvitations;
    }
  } catch {
    return;
  }
}

// loadShareAwaitingUnlockQuietly fills the lock screen's banner; the locked backend reads it with the saved session only.
export async function loadShareAwaitingUnlockQuietly(): Promise<void> {
  if (store.status.state !== "locked") {
    return;
  }
  try {
    const allShareAwaitingUnlock = await api.listAllShareAwaitingUnlock();
    if (store.status.state === "locked") {
      store.allShareAwaitingUnlock = allShareAwaitingUnlock;
    }
  } catch {
    return;
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
    await loadShareInvitationsQuietly();
    await loadAllShareLinkQuietly();
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

export async function copySecretReference(fieldName: string): Promise<void> {
  if (!store.selectedItem) {
    return;
  }
  const itemID = store.selectedItem.id;
  const reference = await runAction(() => api.copySecretReference(store.selectedVaultID, itemID, fieldName));
  if (reference) {
    showNotice(`Copied ${reference} — use it with hisar run or hisar read`);
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
  const result = await runAction(() => Promise.all([api.listAllVaultMember(vaultID), api.listAllShareInvitation()]));
  if (result && vaultID === store.selectedVaultID) {
    [store.allVaultMember, store.shareInvitations] = result;
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

// shareVault sends a share invitation; the server answers the same whether or not the email has an account.
export async function shareVault(email: string, role: VaultRole): Promise<boolean> {
  const vaultID = store.selectedVaultID;
  const shared = await runAction(async () => {
    await api.shareVault(vaultID, email, role);
    return true;
  });
  if (shared) {
    await loadSharing();
    showNotice(`Invitation sent to ${email.trim().toLowerCase()}`);
  }
  return shared === true;
}

export function getAllSentShareInvitation(vaultID: string): SentShareInvitation[] {
  return store.shareInvitations.sent.filter((invitation) => invitation.vaultId === vaultID);
}

export async function acceptShareInvitation(invitation: ReceivedShareInvitation): Promise<boolean> {
  const accepted = await runAction(async () => {
    await api.acceptShareInvitation(invitation.id);
    return true;
  });
  await loadShareInvitationsQuietly();
  if (accepted) {
    showNotice(`Accepted. The vault appears once ${getFirstName({ name: invitation.invitedByName, email: invitation.invitedByEmail })}'s Hisar finishes sharing`);
  }
  return accepted === true;
}

export async function declineShareInvitation(invitation: ReceivedShareInvitation): Promise<boolean> {
  const declined = await runAction(async () => {
    await api.declineShareInvitation(invitation.id);
    return true;
  });
  await loadShareInvitationsQuietly();
  if (declined) {
    showNotice("Invitation declined");
  }
  return declined === true;
}

export async function cancelShareInvitation(invitation: SentShareInvitation): Promise<boolean> {
  const cancelled = await runAction(async () => {
    await api.cancelShareInvitation(invitation.id);
    return true;
  });
  await loadShareInvitationsQuietly();
  if (cancelled) {
    showNotice(`Invitation to ${invitation.email} cancelled`);
  }
  return cancelled === true;
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
  const result = await runAction(() => Promise.all([api.listAllPerson(), api.listAllDevice(), api.listAllShareInvitation()]));
  if (result) {
    [store.allPerson, store.allDevice, store.shareInvitations] = result;
  }
}

// invitePerson asks the server to email a sign-up invite; the answer does not say whether the person has an account.
export async function invitePerson(email: string): Promise<boolean> {
  const invited = await runAction(async () => {
    await api.invitePerson(email);
    return true;
  });
  if (invited) {
    showNotice(`Invitation sent to ${email.trim().toLowerCase()}`);
  }
  return invited === true;
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

export async function setCompactSSHApproval(isCompact: boolean): Promise<void> {
  await changeSSHConfig(() => api.setCompactSSHApproval(isCompact), isCompact ? "SSH approvals now show as a compact card" : "SSH approvals now show the full dialog");
}

export async function setPhoneApproval(isEnabled: boolean): Promise<void> {
  await changeSSHConfig(() => api.setPhoneApproval(isEnabled), isEnabled ? "Your iPhone can now approve requests" : "Requests are approved only on this Mac");
}

export async function setSigningRemember(seconds: number): Promise<void> {
  const notice = seconds > 0 ? "Commit signing dialogs can now remember an app and repository" : "Every commit signature asks again";
  await changeSSHConfig(() => api.setSigningRemember(seconds), notice);
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

export async function setAutoLock(seconds: number): Promise<void> {
  const status = await runAction(() => api.setAutoLock(seconds));
  if (status) {
    store.status = status;
  }
}

export async function setExactHostMatch(isExactHostMatch: boolean): Promise<void> {
  const browserStatus = await runAction(() => api.setExactHostMatch(isExactHostMatch));
  if (browserStatus) {
    store.browserStatus = browserStatus;
  }
}

// Command-line tool actions report errors inside the settings section, like the SSH agent's.
export async function loadCommandLineStatus(): Promise<void> {
  try {
    store.commandLineStatus = await api.getCommandLineStatus();
    store.commandLineErrorMessage = "";
  } catch (error) {
    store.commandLineErrorMessage = getErrorMessage(error);
  }
}

async function changeCommandLine(change: () => Promise<CommandLineStatus>, notice: string): Promise<void> {
  store.isBusy = true;
  try {
    store.commandLineStatus = await change();
    store.commandLineErrorMessage = "";
    showNotice(notice);
  } catch (error) {
    const message = getErrorMessage(error);
    await loadCommandLineStatus();
    store.commandLineErrorMessage = message;
  } finally {
    store.isBusy = false;
  }
}

export async function installCommandLineTool(): Promise<void> {
  await changeCommandLine(() => api.installCommandLineTool(), "Installed the hisar command");
}

export async function uninstallCommandLineTool(): Promise<void> {
  await changeCommandLine(() => api.uninstallCommandLineTool(), "Removed the hisar command");
}

export async function setCommandLineIntegration(isEnabled: boolean): Promise<void> {
  await changeCommandLine(
    () => api.setCommandLineIntegration(isEnabled),
    isEnabled ? "hisar can use Hisar after you approve each terminal" : "hisar signs in on its own now",
  );
}

// Coding agent settings report errors inside their section, like the command-line tool's.
export async function loadAgentVaults(): Promise<void> {
  try {
    store.allAgentVaultID = await api.listAllAgentVaultID();
    store.agentErrorMessage = "";
  } catch (error) {
    store.agentErrorMessage = getErrorMessage(error);
  }
}

export async function setVaultAvailableToAgents(vault: VaultSummary, isAvailable: boolean): Promise<void> {
  store.isBusy = true;
  try {
    store.allAgentVaultID = await api.setVaultAvailableToAgents(vault.id, isAvailable);
    store.agentErrorMessage = "";
    showNotice(isAvailable ? `Coding agents can use ${vault.name} after you approve` : `${vault.name} is invisible to coding agents now`);
  } catch (error) {
    const message = getErrorMessage(error);
    await loadAgentVaults();
    store.agentErrorMessage = message;
  } finally {
    store.isBusy = false;
  }
}

// handleUnlockedElsewhere follows an unlock from the approval window.
export async function handleUnlockedElsewhere(): Promise<void> {
  if (store.status.state !== "unlocked") {
    await loadStatus();
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

export async function loadAllShareLinkQuietly(): Promise<void> {
  try {
    const allShareLink = await api.listAllShareLink();
    if (store.status.state === "unlocked") {
      store.allShareLink = allShareLink;
    }
  } catch {
    return;
  }
}

export function getAllShareLinkForItem(vaultID: string, itemID: string): ShareLink[] {
  return store.allShareLink.filter((shareLink) => shareLink.vaultID === vaultID && shareLink.itemID === itemID);
}

export function hasShareableField(detail: ItemDetail): boolean {
  return Boolean(detail.username || detail.password || detail.notes || detail.urls.length > 0 || detail.fields.some((field) => field.value));
}

export async function openShareLinks(): Promise<void> {
  store.errorMessage = "";
  store.isShareLinksOpen = true;
  const allShareLink = await runAction(() => api.listAllShareLink());
  if (allShareLink) {
    store.allShareLink = allShareLink;
  }
}

export function closeShareLinks(): void {
  store.isShareLinksOpen = false;
  store.errorMessage = "";
}

export async function openItemShareLinkDialog(): Promise<void> {
  const selectedItem = store.selectedItem;
  if (!selectedItem) {
    return;
  }
  const vaultID = store.selectedVaultID;
  const allField = await runAction(() => api.listAllShareLinkField(vaultID, selectedItem.id));
  if (!allField) {
    return;
  }
  if (allField.length === 0) {
    store.errorMessage = "This item has nothing to share";
    return;
  }
  store.shareLinkDialog = { mode: "item", vaultID, itemID: selectedItem.id, title: selectedItem.title, allField };
}

export function openTextShareLinkDialog(): void {
  store.errorMessage = "";
  store.shareLinkDialog = { mode: "text", vaultID: "", itemID: "", title: "", allField: [] };
}

export function closeShareLinkDialog(): void {
  store.shareLinkDialog = null;
  store.errorMessage = "";
}

export async function createShareLink(request: ShareLinkRequest): Promise<ShareLink | undefined> {
  const created = await runAction(() => api.createShareLink(request));
  if (created) {
    await loadAllShareLinkQuietly();
  }
  return created;
}

export async function copyShareLink(shareLink: ShareLink): Promise<void> {
  const copied = await runAction(async () => {
    await api.copyShareLink(shareLink.id);
    return true;
  });
  if (copied) {
    showNotice("Link copied, clears in 90 seconds");
  }
}

export async function revokeShareLink(shareLink: ShareLink): Promise<boolean> {
  const revoked = await runAction(async () => {
    await api.revokeShareLink(shareLink.id);
    return true;
  });
  await loadAllShareLinkQuietly();
  if (revoked) {
    showNotice(`Link to ${shareLink.title} revoked`);
  }
  return revoked === true;
}

// getShownPlanLimit is the limit behind the error on screen, so the notice disappears with the error.
export function getShownPlanLimit(): PlanLimit | null {
  return store.planLimit && store.errorMessage && store.errorMessage === store.planLimit.message ? store.planLimit : null;
}

export async function loadPlan(): Promise<Plan | null> {
  try {
    store.plan = await api.getPlan();
    store.isPlanUnavailable = false;
  } catch {
    store.isPlanUnavailable = true;
  }
  return store.plan;
}

// openCheckout opens the checkout in the browser and returns an error message, empty when it worked.
export async function openCheckout(interval: "monthly" | "yearly"): Promise<string> {
  try {
    let checkoutURL = pricingURL;
    try {
      checkoutURL = await api.createCheckoutURL(interval);
    } catch (error) {
      if (getErrorMessage(error) !== "plans are not sold on this server") {
        throw error;
      }
    }
    await api.openURL(checkoutURL);
    return "";
  } catch (error) {
    return getErrorMessage(error);
  }
}

export async function openManageSubscription(): Promise<string> {
  try {
    await api.openURL(await api.createManageSubscriptionURL());
    return "";
  } catch (error) {
    return getErrorMessage(error);
  }
}
