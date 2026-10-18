import { reactive } from "vue";
import {
  type AccountDeletionPreview,
  type CredentialChangeResult,
  api,
  getErrorMessage,
  type BrowserPairingRequest,
  type BrowserStatus,
  type CommandLineStatus,
  type Device,
  type LauncherApp,
  type Organization,
  type OrganizationInvitation,
  type OrganizationMember,
  type OrganizationRole,
  type Organizations,
  type OrganizationSummary,
  type Person,
  type SentOrganizationInvitation,
  type Team,
  type TeamMember,
  type TeamVault,
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
  type UpdateRequirement,
  type UpdateStatus,
  type VaultMember,
  type VaultRole,
  type VaultSummary,
  type VaultActions,
  type RemovedVaultNotice,
  type VaultDeletionRequest,
  type ExportRequest,
  type ExportResult,
  type FavoriteItem,
  type WaitingVaultKeys,
} from "./api";
import { allAddressKey, formatDatabaseLocation, formatFullName, formatRoleWithArticle, formatServerAddress } from "./util";
import { forgetWebsiteIcons } from "./websiteicons";

export type EditorMode = "view" | "edit" | "create" | "create-secure-note" | "create-credit-card" | "create-identity" | "create-api-credential" | "create-address" | "create-software-license" | "create-server" | "create-database" | "create-ssh-key";
export type ImportStep = "choose" | "preview" | "saving" | "done";

export interface ShareLinkDialog {
  mode: "item" | "text";
  vaultID: string;
  itemID: string;
  title: string;
  allField: ShareLinkField[];
}

export type VaultDialogMode = "rename" | "delete" | "leave" | "blocked";

// VaultDialog: blockedReason explains why the chosen action is not possible, in the "blocked" mode.
export interface VaultDialog {
  mode: VaultDialogMode;
  actions: VaultActions;
  blockedReason: string;
}

// OrganizationAlert: title names the refused action, message is the server's sentence.
export interface OrganizationAlert {
  title: string;
  message: string;
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
    updateRequired: null,
  } as Status,
  allVault: [] as VaultSummary[],
  selectedVaultID: "",
  allItem: [] as ItemDetail[],
  selectedItem: null as ItemDetail | null,
  // allFavorite holds the user's starred items of every vault; isFavoritesSelected shows them in the list instead of a vault.
  allFavorite: [] as FavoriteItem[],
  isFavoritesSelected: false,
  editorMode: "view" as EditorMode,
  searchQuery: "",
  // selectedTag narrows the list to the items carrying it; "" shows every item.
  selectedTag: "",
  pendingSecretKey: "",
  isTouchIDOfferShown: false,
  pendingInvite: "",
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
  allLauncherApp: [] as LauncherApp[],
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
  vaultDialog: null as VaultDialog | null,
  allRemovedVaultNotice: [] as RemovedVaultNotice[],
  organizations: { organizations: [], invitations: [] } as Organizations,
  // organizationByID holds the organizations this window opened: in Settings, an organization vault's sharing panel and Where.
  organizationByID: {} as Record<string, Organization>,
  // selectedOrganizationID is the organization Settings → Organizations shows; "" shows the list.
  selectedOrganizationID: "",
  organizationsErrorMessage: "",
  waitingVaultKeys: { waiting: [], deliveryError: "", rotationError: "" } as WaitingVaultKeys,
  // organizationAlert is a refused organization change: the action and the server's sentence.
  organizationAlert: null as OrganizationAlert | null,
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
  forgetWebsiteIcons();
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
  store.vaultDialog = null;
  store.allRemovedVaultNotice = [];
  store.organizations = { organizations: [], invitations: [] };
  store.organizationByID = {};
  store.selectedOrganizationID = "";
  store.organizationsErrorMessage = "";
  store.waitingVaultKeys = { waiting: [], deliveryError: "", rotationError: "" };
  store.organizationAlert = null;
  store.allVault = [];
  store.selectedVaultID = "";
  store.allItem = [];
  store.selectedItem = null;
  store.allFavorite = [];
  store.isFavoritesSelected = false;
  store.editorMode = "view";
  store.searchQuery = "";
  store.selectedTag = "";
}

async function applyStatus(status: Status): Promise<void> {
  store.status = status;
  if (status.state === "unlocked" && !status.updateRequired) {
    await loadAllVault();
  } else {
    clearAllVaultData();
  }
}

export async function loadStatus(): Promise<void> {
  await runAction(async () => applyStatus(await api.getStatus()));
}

export function showUpdateRequired(requirement: UpdateRequirement): void {
  clearAllVaultData();
  store.status = { ...store.status, updateRequired: requirement };
}

export async function checkMinimumVersion(): Promise<boolean> {
  const status = await runAction(() => api.checkMinimumVersion());
  if (status) {
    await applyStatus(status);
  }
  return !store.status.updateRequired;
}

// updateNow installs through Sparkle when it can; development builds and signed-out apps get the download page.
export async function updateNow(): Promise<void> {
  const updateURL = store.status.updateRequired?.updateURL ?? "";
  const updateStatus = await api.getUpdateStatus().catch(() => null);
  if (updateStatus?.isAvailable && updateStatus.isSignedIn) {
    await runAction(() => api.checkForUpdates());
    return;
  }
  if (/^https:\/\//.test(updateURL)) {
    await runAction(() => api.openURL(updateURL));
  }
}

export function openInviteLink(invite: string): void {
  if (!invite) {
    return;
  }
  if (store.status.state !== "signed-out") {
    showNotice(`You're signed in as ${store.status.email}. Sign out to create an account with this invite.`);
    return;
  }
  store.pendingInvite = invite;
}

export async function takePendingInviteQuietly(): Promise<void> {
  try {
    openInviteLink(await api.takePendingInvite());
  } catch {
    return;
  }
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

export async function changeMasterPassword(currentPassword: string, newPassword: string): Promise<CredentialChangeResult | undefined> {
  const result = await runAction(() => api.changeMasterPassword(currentPassword, newPassword));
  if (result) {
    store.status = result.status;
  }
  return result;
}

export async function replaceSecretKey(password: string): Promise<CredentialChangeResult | undefined> {
  const result = await runAction(() => api.replaceSecretKey(password));
  if (result) {
    store.status = result.status;
  }
  return result;
}

export async function finishSecretKeyReplacement(): Promise<void> {
  await api.forgetNewAccountSecretKey().catch(() => undefined);
}

export async function loadAccountDeletionPreview(): Promise<AccountDeletionPreview | undefined> {
  return runAction(() => api.previewAccountDeletion());
}

export async function deleteAccount(password: string, email: string): Promise<boolean> {
  const status = await runAction(() => api.deleteAccount(password, email));
  if (status) {
    store.pendingSecretKey = "";
    await applyStatus(status);
    showNotice("Your account was deleted");
  }
  return status !== undefined;
}

export async function dismissSignedOutReason(): Promise<void> {
  try {
    store.status = await api.dismissSignedOutReason();
  } catch {
    store.status = { ...store.status, signedOutReason: "" };
  }
}

export async function loadAllVault(): Promise<void> {
  const allVault = await runAction(() => api.listAllVault());
  if (!allVault) {
    return;
  }
  store.allVault = allVault;
  await loadRemovedVaultNoticeQuietly();
  const stillExists = allVault.some((vault) => vault.id === store.selectedVaultID);
  if (!stillExists && allVault.length > 0) {
    await selectVault(findPersonalVaultID(allVault));
  }
  await loadShareInvitationsQuietly();
  await loadAllShareLinkQuietly();
  await loadAllFavoriteQuietly();
  await refreshOrganizationsQuietly();
}

function findPersonalVaultID(allVault: VaultSummary[]): string {
  return (allVault.find((vault) => vault.type === "personal") ?? allVault[0]).id;
}

// loadRemovedVaultNoticeQuietly shows vaults another person deleted or removed this user from; each notice comes once.
export async function loadRemovedVaultNoticeQuietly(): Promise<void> {
  try {
    const allNotice = await api.listAllRemovedVault();
    if (allNotice.length > 0 && store.status.state === "unlocked") {
      store.allRemovedVaultNotice = [...store.allRemovedVaultNotice, ...allNotice];
    }
  } catch {
    return;
  }
}

export function dismissRemovedVaultNotice(vaultID: string): void {
  store.allRemovedVaultNotice = store.allRemovedVaultNotice.filter((notice) => notice.id !== vaultID);
}

export async function openVaultDialog(vaultID: string, mode: Exclude<VaultDialogMode, "blocked">): Promise<void> {
  const actions = await runAction(() => api.getVaultActions(vaultID));
  if (!actions) {
    return;
  }
  let blockedReason = "";
  if (mode === "delete" && !actions.canDelete) {
    blockedReason = actions.deleteBlockedReason;
  } else if (mode === "leave" && !actions.canLeave) {
    blockedReason = actions.leaveBlockedReason;
  } else if (mode === "rename" && !actions.canRename) {
    blockedReason = "Only an owner of this vault can rename it.";
  }
  store.vaultDialog = { mode: blockedReason ? "blocked" : mode, actions, blockedReason };
}

export function closeVaultDialog(): void {
  store.vaultDialog = null;
  store.errorMessage = "";
}

export async function renameVault(name: string): Promise<boolean> {
  const vaultID = store.vaultDialog?.actions.vault.id ?? "";
  const renamed = await runAction(() => api.renameVault(vaultID, name));
  if (!renamed) {
    return false;
  }
  store.vaultDialog = null;
  store.allVault = store.allVault.map((vault) => (vault.id === vaultID ? { ...vault, name: renamed.name } : vault));
  showNotice(`Renamed the vault to ${renamed.name}`);
  return true;
}

export async function deleteVault(request: VaultDeletionRequest): Promise<boolean> {
  const vault = store.vaultDialog?.actions.vault;
  if (!vault) {
    return false;
  }
  const result = await runAction(() => api.deleteVault(vault.id, request));
  if (!result) {
    return false;
  }
  store.vaultDialog = null;
  await loadAllVault();
  await selectVault(request.moveToVaultId || findPersonalVaultID(store.allVault));
  if (request.moveToVaultId) {
    const movedText = result.movedCount === 1 ? "1 item" : `${result.movedCount} items`;
    showNotice(`Moved ${movedText} to ${result.targetVaultName} and deleted ${vault.name}.`);
  } else {
    showNotice(`Deleted ${vault.name}.`);
  }
  return true;
}

export async function leaveVault(): Promise<boolean> {
  const vault = store.vaultDialog?.actions.vault;
  if (!vault) {
    return false;
  }
  const left = await runAction(async () => {
    await api.leaveVault(vault.id);
    return true;
  });
  if (!left) {
    return false;
  }
  store.vaultDialog = null;
  await loadAllVault();
  await selectVault(findPersonalVaultID(store.allVault));
  showNotice(`You left ${vault.name}.`);
  return true;
}

// exportAllData returns undefined when the export failed or the save dialog was cancelled.
export async function exportAllData(request: ExportRequest): Promise<ExportResult | undefined> {
  const result = await runAction(() => api.exportAllData(request));
  return result && result.path ? result : undefined;
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
    await loadRemovedVaultNoticeQuietly();
    await loadShareInvitationsQuietly();
    await loadAllShareLinkQuietly();
    await loadAllFavoriteQuietly();
    await refreshOrganizationsQuietly();
    if (!allVault.some((vault) => vault.id === store.selectedVaultID)) {
      if (allVault.length > 0) {
        await selectVault(findPersonalVaultID(allVault));
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
  store.isFavoritesSelected = false;
  store.selectedVaultID = vaultID;
  store.selectedItem = null;
  store.editorMode = "view";
  store.searchQuery = "";
  store.selectedTag = "";
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

// createOrganizationVault creates a vault the team owns; this app gives the team's other members its key.
export async function createOrganizationVault(organizationID: string, teamID: string, name: string): Promise<boolean> {
  const vault = await runAction(() => api.createOrganizationVault(organizationID, teamID, name));
  if (!vault) {
    return false;
  }
  await loadAllVault();
  await selectVault(vault.id);
  return true;
}

export async function loadAllFavoriteQuietly(): Promise<void> {
  try {
    store.allFavorite = await api.listAllFavoriteItem();
  } catch {
    return;
  }
}

// selectFavorites lists the starred items of every vault; picking one makes its vault the one item actions use.
export async function selectFavorites(): Promise<void> {
  store.isFavoritesSelected = true;
  store.selectedItem = null;
  store.editorMode = "view";
  store.searchQuery = "";
  const allFavorite = await runAction(() => api.listAllFavoriteItem());
  if (allFavorite) {
    store.allFavorite = allFavorite;
  }
}

export async function selectFavoriteItem(favorite: FavoriteItem): Promise<void> {
  const detail = await runAction(() => api.getItem(favorite.vaultId, favorite.item.id));
  if (detail) {
    store.selectedVaultID = favorite.vaultId;
    store.selectedItem = detail;
    store.editorMode = "view";
  }
}

export function filterAllFavorite(allFavorite: FavoriteItem[], query: string): FavoriteItem[] {
  const allVisibleItem = filterAllItem(
    allFavorite.map((favorite) => favorite.item),
    query,
  );
  return allFavorite.filter((favorite) => allVisibleItem.includes(favorite.item));
}

// setItemFavorite stars or unstars an item of vaultID for this user; the people they share the vault with don't see it.
export async function setItemFavorite(vaultID: string, itemID: string, isFavorite: boolean): Promise<void> {
  const isSaved = await runAction(async () => {
    await api.setItemFavorite(vaultID, itemID, isFavorite);
    return true;
  });
  if (!isSaved) {
    return;
  }
  if (store.selectedItem?.id === itemID) {
    store.selectedItem = { ...store.selectedItem, isFavorite };
  }
  if (vaultID === store.selectedVaultID) {
    store.allItem = store.allItem.map((vaultItem) => (vaultItem.id === itemID ? { ...vaultItem, isFavorite } : vaultItem));
  }
  await loadAllFavoriteQuietly();
  showNotice(isFavorite ? "Added to Favorites" : "Removed from Favorites");
}

export async function toggleSelectedItemFavorite(): Promise<void> {
  if (store.selectedItem && store.editorMode === "view") {
    await setItemFavorite(store.selectedVaultID, store.selectedItem.id, !store.selectedItem.isFavorite);
  }
}

export function sortAllItem(allItem: ItemDetail[]): ItemDetail[] {
  return [...allItem].sort((first, second) => first.title.localeCompare(second.title, undefined, { sensitivity: "base" }));
}

// filterAllItem keeps the items matching the search in their title, username, websites, tags, identity, API credential,
// address, software license, server or database summary, and carrying tag when one is selected.
export function filterAllItem(allItem: ItemDetail[], query: string, tag = ""): ItemDetail[] {
  const normalizedQuery = query.trim().toLowerCase();
  return allItem.filter((vaultItem) => {
    const allTag = vaultItem.tags ?? [];
    // An identity's list summary keeps its name, email and address, an API credential's its service, key ID and host, an
    // address's first street line, city and country. Lists show some of them under the title; search reads them all, so an
    // identity imported with an address is found by its city.
    const identity = vaultItem.identity;
    const identityText = identity ? [formatFullName(identity), identity.email, identity.street, identity.city, identity.country] : [];
    const credential = vaultItem.apiCredential;
    const credentialText = credential ? [credential.service, credential.keyId, credential.host] : [];
    const address = vaultItem.address;
    const addressText = address ? [address.street, address.city, address.country] : [];
    // A software license's summary keeps its product, version, licensed-to name and email, never the key.
    const license = vaultItem.softwareLicense;
    const licenseText = license ? [license.product, license.version, license.licensedTo, license.email] : [];
    // A server's summary keeps its host and port, which lists show as one address; its username is the item's own.
    const server = vaultItem.server;
    const serverText = server ? [formatServerAddress(server), server.host, server.port] : [];
    // A database's summary keeps its engine, address and name; never the connection string, which holds the password.
    const database = vaultItem.database;
    const databaseText = database ? [database.engineName, formatDatabaseLocation(database), database.host, database.port, database.name] : [];
    return (
      (tag === "" || allTag.includes(tag)) &&
      (normalizedQuery === "" ||
        [vaultItem.title, vaultItem.username, ...vaultItem.urls, ...allTag, ...identityText, ...credentialText, ...addressText, ...licenseText, ...serverText, ...databaseText].some((text) =>
          text.toLowerCase().includes(normalizedQuery),
        ))
    );
  });
}

export interface TagSummary {
  name: string;
  itemCount: number;
}

// listAllTag returns every tag of the items with how many carry it, sorted by name.
export function listAllTag(allItem: ItemDetail[]): TagSummary[] {
  const countByTag = new Map<string, number>();
  for (const vaultItem of allItem) {
    for (const tag of vaultItem.tags ?? []) {
      countByTag.set(tag, (countByTag.get(tag) ?? 0) + 1);
    }
  }
  return [...countByTag]
    .map(([name, itemCount]) => ({ name, itemCount }))
    .sort((first, second) => first.name.localeCompare(second.name, undefined, { sensitivity: "base" }));
}

// selectTag filters the list by the tag, and clears the filter when the tag is already selected.
export function selectTag(tag: string): void {
  store.selectedTag = store.selectedTag === tag ? "" : tag;
  // The filter narrows the selected vault's list, so choosing a tag from Favorites returns to that list.
  if (store.isFavoritesSelected) {
    store.isFavoritesSelected = false;
    store.selectedItem = null;
    store.editorMode = "view";
  }
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

export function startCreatingSecureNote(): void {
  store.selectedItem = null;
  store.editorMode = "create-secure-note";
}

export function startCreatingCard(): void {
  store.selectedItem = null;
  store.editorMode = "create-credit-card";
}

export function startCreatingIdentity(): void {
  store.selectedItem = null;
  store.editorMode = "create-identity";
}

export function startCreatingAPICredential(): void {
  store.selectedItem = null;
  store.editorMode = "create-api-credential";
}

export function startCreatingAddress(): void {
  store.selectedItem = null;
  store.editorMode = "create-address";
}

export function startCreatingSoftwareLicense(): void {
  store.selectedItem = null;
  store.editorMode = "create-software-license";
}

export function startCreatingServer(): void {
  store.selectedItem = null;
  store.editorMode = "create-server";
}

export function startCreatingDatabase(): void {
  store.selectedItem = null;
  store.editorMode = "create-database";
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
    dropMissingTagFilter();
  }
  store.selectedItem = saved;
  store.editorMode = "view";
  if (saved.isFavorite) {
    await loadAllFavoriteQuietly();
  }
  showNotice("Saved");
  return true;
}

// dropMissingTagFilter clears the tag filter once no item carries the tag any more, so the list never stays empty behind
// a tag the sidebar no longer shows.
function dropMissingTagFilter(): void {
  if (store.selectedTag !== "" && !store.allItem.some((vaultItem) => (vaultItem.tags ?? []).includes(store.selectedTag))) {
    store.selectedTag = "";
  }
}

export async function convertToSecureNote(): Promise<void> {
  if (!store.selectedItem) {
    return;
  }
  const converted = await runAction(() => api.convertToSecureNote(store.selectedVaultID, store.selectedItem?.id ?? ""));
  if (!converted) {
    return;
  }
  store.allItem = store.allItem.map((vaultItem) => (vaultItem.id === converted.id ? { ...vaultItem, category: converted.category, revision: converted.revision, canConvertToSecureNote: false } : vaultItem));
  if (store.selectedItem?.id === converted.id) {
    store.selectedItem = converted;
  }
  if (converted.isFavorite) {
    await loadAllFavoriteQuietly();
  }
  showNotice("Converted to a secure note");
}

// deleteItem deletes an item of vaultID, the selected vault unless the item comes from another vault's favorite.
export async function deleteItem(itemID: string, vaultID = store.selectedVaultID): Promise<boolean> {
  const deleted = await runAction(async () => {
    await api.deleteItem(vaultID, itemID);
    return true;
  });
  if (!deleted) {
    return false;
  }
  store.allItem = store.allItem.filter((vaultItem) => vaultItem.id !== itemID);
  // Opening a favorite makes its vault the selected one but leaves allItem as the vault last listed, so allItem counts the
  // selected vault only outside Favorites.
  if (vaultID === store.selectedVaultID && !store.isFavoritesSelected) {
    updateSelectedVaultItemCount();
    dropMissingTagFilter();
  } else {
    const vault = store.allVault.find((candidate) => candidate.id === vaultID);
    if (vault) {
      vault.itemCount = Math.max(0, vault.itemCount - 1);
    }
  }
  store.allFavorite = store.allFavorite.filter((favorite) => !(favorite.item.id === itemID && favorite.vaultId === vaultID));
  if (store.selectedItem?.id === itemID) {
    store.selectedItem = null;
    store.editorMode = "view";
  }
  showNotice("Deleted");
  return true;
}

// reloadSelectedVaultItems lists the vault on screen again, as summaries without secrets.
async function reloadSelectedVaultItems(): Promise<void> {
  const allItem = await runAction(() => api.listAllItem(store.selectedVaultID));
  if (allItem) {
    store.allItem = sortAllItem(allItem);
  }
}

// moveItem moves an item of vaultID to targetVaultID; it keeps its ID, so it stays a favorite.
export async function moveItem(vaultID: string, itemID: string, targetVaultID: string): Promise<boolean> {
  const moved = await runAction(() => api.moveItem(vaultID, itemID, targetVaultID));
  if (!moved) {
    return false;
  }
  if (vaultID === store.selectedVaultID) {
    store.allItem = store.allItem.filter((vaultItem) => vaultItem.id !== itemID);
    dropMissingTagFilter();
  } else if (targetVaultID === store.selectedVaultID) {
    await reloadSelectedVaultItems();
  }
  for (const vault of store.allVault) {
    if (vault.id === vaultID) {
      vault.itemCount = Math.max(0, vault.itemCount - 1);
    } else if (vault.id === targetVaultID) {
      vault.itemCount += 1;
    }
  }
  if (store.selectedItem?.id === itemID) {
    store.selectedItem = null;
    store.editorMode = "view";
  }
  await loadAllFavoriteQuietly();
  const targetName = store.allVault.find((vault) => vault.id === targetVaultID)?.name ?? "the vault";
  showNotice(`Moved ${moved.title} to ${targetName}`);
  return true;
}

// duplicateItem saves a copy of the item in its vault and opens the copy when that vault is on screen.
export async function duplicateItem(vaultID: string, itemID: string): Promise<boolean> {
  const copied = await runAction(() => api.duplicateItem(vaultID, itemID));
  if (!copied) {
    return false;
  }
  const vault = store.allVault.find((candidate) => candidate.id === vaultID);
  if (vault) {
    vault.itemCount += 1;
  }
  if (vaultID === store.selectedVaultID && !store.isFavoritesSelected) {
    await reloadSelectedVaultItems();
    store.selectedItem = copied;
    store.editorMode = "view";
  }
  showNotice(`Duplicated as ${copied.title}`);
  return true;
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
  if (store.selectedItem) {
    await copyItemField(store.selectedVaultID, store.selectedItem.id, fieldName, label);
  }
}

// copyItemField copies a field of any vault's item, such as a favorite's from another vault, without selecting it.
export async function copyItemField(vaultID: string, itemID: string, fieldName: string, label: string): Promise<void> {
  const copied = await runAction(async () => {
    await api.copyField(vaultID, itemID, fieldName);
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
  const organizationID = getSelectedVault()?.organizationId;
  if (organizationID) {
    const organization = await runAction(() => api.getOrganization(organizationID));
    if (organization) {
      store.organizationByID = { ...store.organizationByID, [organizationID]: organization };
    }
    return;
  }
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

// Organizations load quietly for the vault list and its banners; Settings → Organizations says why its list could not
// load, like the SSH agent section. A refused change shows the server's sentence in an alert named after the action.

function applyOrganizations(organizations: Organizations): void {
  store.organizations = organizations;
  const allOrganizationID = new Set(organizations.organizations.map((organization) => organization.id));
  store.organizationByID = Object.fromEntries(Object.entries(store.organizationByID).filter(([organizationID]) => allOrganizationID.has(organizationID)));
  if (!allOrganizationID.has(store.selectedOrganizationID)) {
    store.selectedOrganizationID = "";
  }
}

async function loadOrganizationQuietly(organizationID: string): Promise<void> {
  try {
    const organization = await api.getOrganization(organizationID);
    if (store.status.state === "unlocked") {
      store.organizationByID = { ...store.organizationByID, [organizationID]: organization };
    }
  } catch {
    return;
  }
}

// refreshOrganizationsQuietly reloads the organizations with the invitations to them, the vaults waiting for a key (the
// backend first gives and re-encrypts what this app can), every organization this window loaded (Settings, a sharing
// panel, the new-vault form's Where) and the organizations on screen.
export async function refreshOrganizationsQuietly(): Promise<void> {
  try {
    const organizations = await api.listAllOrganization();
    if (store.status.state !== "unlocked") {
      return;
    }
    applyOrganizations(organizations);
    const waitingVaultKeys = await api.listAllWaitingVaultKey();
    if (store.status.state !== "unlocked") {
      return;
    }
    store.waitingVaultKeys = waitingVaultKeys;
  } catch {
    return;
  }
  const allReloadedOrganizationID = new Set(Object.keys(store.organizationByID));
  if (store.isSettingsOpen && store.selectedOrganizationID) {
    allReloadedOrganizationID.add(store.selectedOrganizationID);
  }
  const sharedOrganizationID = getSelectedVault()?.organizationId;
  if (store.isSharingOpen && sharedOrganizationID && store.organizations.organizations.some((organization) => organization.id === sharedOrganizationID)) {
    allReloadedOrganizationID.add(sharedOrganizationID);
  }
  for (const organizationID of allReloadedOrganizationID) {
    await loadOrganizationQuietly(organizationID);
  }
}

// loadOrganizations fills Settings → Organizations and keeps the reason there when the list could not load.
export async function loadOrganizations(): Promise<void> {
  try {
    const organizations = await api.listAllOrganization();
    if (store.status.state === "unlocked") {
      applyOrganizations(organizations);
      store.organizationsErrorMessage = "";
    }
  } catch (error) {
    store.organizationsErrorMessage = getErrorMessage(error);
  }
}

// loadAllOrganizationQuietly loads every organization with its teams, for the new-vault form's Where.
export async function loadAllOrganizationQuietly(): Promise<void> {
  try {
    const organizations = await api.listAllOrganization();
    if (store.status.state !== "unlocked") {
      return;
    }
    applyOrganizations(organizations);
  } catch {
    return;
  }
  for (const organization of store.organizations.organizations) {
    await loadOrganizationQuietly(organization.id);
  }
}

export async function openOrganization(organizationID: string): Promise<void> {
  store.selectedOrganizationID = organizationID;
  const organization = await runAction(() => api.getOrganization(organizationID));
  if (organization) {
    store.organizationByID = { ...store.organizationByID, [organizationID]: organization };
  }
}

async function reloadAllVaultQuietly(): Promise<void> {
  try {
    const allVault = await api.listAllVault();
    if (store.status.state !== "unlocked") {
      return;
    }
    store.allVault = allVault;
    if (allVault.length > 0 && !allVault.some((vault) => vault.id === store.selectedVaultID)) {
      await selectVault(findPersonalVaultID(allVault));
    }
  } catch {
    return;
  }
}

// changeOrganization runs one change and refreshes the organizations on screen; isChangingAccess also reloads the vault
// list, as the change can give or take vaults and roles.
async function changeOrganization(title: string, change: () => Promise<unknown>, notice: string, isChangingAccess = false): Promise<boolean> {
  const isChanged = await runAction(async () => {
    await change();
    return true;
  });
  if (!isChanged && store.status.state === "unlocked") {
    store.organizationAlert = { title, message: store.errorMessage };
    store.errorMessage = "";
  }
  await refreshOrganizationsQuietly();
  if (isChanged && isChangingAccess) {
    await reloadAllVaultQuietly();
  }
  if (isChanged && notice) {
    showNotice(notice);
  }
  return isChanged === true;
}

export async function createOrganization(name: string): Promise<boolean> {
  let created: OrganizationSummary | undefined;
  const isCreated = await changeOrganization("Create organization", async () => (created = await api.createOrganization(name)), "");
  if (created) {
    await openOrganization(created.id);
  }
  return isCreated;
}

export async function renameOrganization(organization: Organization, name: string): Promise<boolean> {
  return changeOrganization("Rename organization", () => api.renameOrganization(organization.id, name), `Renamed the organization to ${name.trim()}`);
}

export async function deleteOrganization(organization: Organization): Promise<boolean> {
  return changeOrganization("Delete organization", () => api.deleteOrganization(organization.id), `Deleted ${organization.name}.`);
}

export async function leaveOrganization(organization: Organization): Promise<boolean> {
  return changeOrganization(`Leave ${organization.name}`, () => api.leaveOrganization(organization.id), `You left ${organization.name}.`, true);
}

// inviteToOrganization: the server answers the same whether or not the email has an account.
export async function inviteToOrganization(organization: Organization, email: string, role: OrganizationRole): Promise<boolean> {
  return changeOrganization("Send invitation", () => api.inviteToOrganization(organization.id, email, role), `Invitation sent to ${email.trim().toLowerCase()}`);
}

export async function cancelOrganizationInvitation(organization: Organization, invitation: SentOrganizationInvitation): Promise<boolean> {
  return changeOrganization("Cancel invitation", () => api.cancelOrganizationInvitation(organization.id, invitation.id), `Invitation to ${invitation.email} cancelled`);
}

export async function setOrganizationMemberRole(organization: Organization, member: OrganizationMember, role: OrganizationRole): Promise<boolean> {
  return changeOrganization(
    role === "admin" ? "Make admin" : "Make member",
    () => api.setOrganizationMemberRole(organization.id, member.accountId, role),
    `${getFirstName(member)} is now ${formatRoleWithArticle(role)}`,
  );
}

export async function removeOrganizationMember(organization: Organization, member: OrganizationMember): Promise<boolean> {
  return changeOrganization(
    `Remove from ${organization.name}`,
    () => api.removeOrganizationMember(organization.id, member.accountId),
    `Removed ${getFirstName(member)} from ${organization.name}`,
    true,
  );
}

export async function createTeam(organization: Organization, name: string): Promise<boolean> {
  return changeOrganization("New team", () => api.createTeam(organization.id, name), "");
}

export async function renameTeam(organization: Organization, team: Team, name: string): Promise<boolean> {
  return changeOrganization("Rename team", () => api.renameTeam(organization.id, team.id, name), `Renamed the team to ${name.trim()}`);
}

export async function deleteTeam(organization: Organization, team: Team): Promise<boolean> {
  return changeOrganization("Delete team", () => api.deleteTeam(organization.id, team.id), `Deleted ${team.name}.`, true);
}

export async function addTeamMember(organization: Organization, team: Team, member: OrganizationMember): Promise<boolean> {
  return changeOrganization("Add member", () => api.addTeamMember(organization.id, team.id, member.accountId), `Added ${getFirstName(member)} to ${team.name}`, true);
}

export async function removeTeamMember(organization: Organization, team: Team, member: TeamMember): Promise<boolean> {
  return changeOrganization("Remove", () => api.removeTeamMember(organization.id, team.id, member.accountId), `Removed ${getFirstName(member)} from ${team.name}`, true);
}

// giveVaultToTeam gives an organization vault to a team, or changes the role the team has it with.
export async function giveVaultToTeam(organization: Organization, team: Team, vaultID: string, role: VaultRole): Promise<boolean> {
  const isChange = team.vaults.some((teamVault) => teamVault.vaultId === vaultID);
  return changeOrganization(
    isChange ? "Change role" : "Give",
    () => api.giveVaultToTeam(organization.id, team.id, vaultID, role),
    isChange ? `${team.name} is now ${formatRoleWithArticle(role)} of the vault` : `Gave the vault to ${team.name}`,
    true,
  );
}

export async function takeVaultFromTeam(organization: Organization, team: Team, teamVault: TeamVault): Promise<boolean> {
  return changeOrganization(
    "Take back",
    () => api.takeVaultFromTeam(organization.id, team.id, teamVault.vaultId),
    `Took ${teamVault.canOpen ? teamVault.name : "the vault"} back from ${team.name}`,
    true,
  );
}

export async function acceptOrganizationInvitation(invitation: OrganizationInvitation): Promise<boolean> {
  const accepted = await runAction(async () => {
    await api.acceptOrganizationInvitation(invitation.id);
    return true;
  });
  await refreshOrganizationsQuietly();
  if (accepted) {
    showNotice(`You joined ${invitation.organizationName}. Its vaults appear once an admin adds you to a team.`);
  }
  return accepted === true;
}

export async function declineOrganizationInvitation(invitation: OrganizationInvitation): Promise<boolean> {
  const declined = await runAction(async () => {
    await api.declineOrganizationInvitation(invitation.id);
    return true;
  });
  await refreshOrganizationsQuietly();
  if (declined) {
    showNotice("Invitation declined");
  }
  return declined === true;
}

// describeTeamVault names a team's vault, or says this device has no key for it.
export function describeTeamVault(teamVault: TeamVault): string {
  return teamVault.canOpen ? teamVault.name : "A vault you can't open";
}

// OrganizationVault is a vault of an organization with the names of the teams that have it; name is "" when this
// device has no key for it.
export interface OrganizationVault {
  vaultId: string;
  name: string;
  canOpen: boolean;
  allTeamName: string[];
}

// listAllOrganizationVault lists the vaults the organization's teams have and those this device opens, named first.
export function listAllOrganizationVault(organization: Organization): OrganizationVault[] {
  const vaultByID = new Map<string, OrganizationVault>();
  for (const team of organization.teams) {
    for (const teamVault of team.vaults) {
      const organizationVault = vaultByID.get(teamVault.vaultId) ?? { vaultId: teamVault.vaultId, name: teamVault.name, canOpen: teamVault.canOpen, allTeamName: [] };
      organizationVault.allTeamName.push(team.name);
      vaultByID.set(teamVault.vaultId, organizationVault);
    }
  }
  for (const vault of store.allVault) {
    if (vault.organizationId === organization.id && !vaultByID.has(vault.id)) {
      vaultByID.set(vault.id, { vaultId: vault.id, name: vault.name, canOpen: true, allTeamName: [] });
    }
  }
  return [...vaultByID.values()].sort(
    (first, second) => Number(second.canOpen) - Number(first.canOpen) || first.name.localeCompare(second.name, undefined, { sensitivity: "base" }),
  );
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

export async function setSSHRemember(seconds: number): Promise<void> {
  const notice = seconds < 0 ? "Remembered SSH approvals now last until Hisar locks" : "Remembered SSH approvals now last 1 hour";
  await changeSSHConfig(() => api.setSSHRemember(seconds), notice);
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

// setCommandLineIntegration reloads the launchers too: turning the integration off ends every approval.
export async function setCommandLineIntegration(isEnabled: boolean): Promise<void> {
  await changeCommandLine(
    () => api.setCommandLineIntegration(isEnabled),
    isEnabled ? "hisar can use Hisar after you approve each terminal" : "hisar signs in on its own now",
  );
  await loadLauncherApps();
}

export async function loadLauncherApps(): Promise<void> {
  try {
    store.allLauncherApp = await api.listAllLauncherApp();
  } catch (error) {
    store.commandLineErrorMessage = getErrorMessage(error);
  }
}

export async function revokeLauncherApp(launcherApp: LauncherApp): Promise<void> {
  store.isBusy = true;
  try {
    store.allLauncherApp = await api.revokeLauncherApp(launcherApp.path);
    store.commandLineErrorMessage = "";
    showNotice(`${launcherApp.name} asks again the next time it uses Hisar`);
  } catch (error) {
    store.commandLineErrorMessage = getErrorMessage(error);
  } finally {
    store.isBusy = false;
  }
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
  await loadAllFavoriteQuietly();
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
  // A card's, an identity's, an API credential's, an address's and a software license's values come in card, identity,
  // apiCredential, address and softwareLicense, not in fields.
  const card = detail.card;
  const allCardValue = card ? [card.cardholder, card.type, card.number, card.expiry, card.securityCode, card.pin, card.billingAddress] : [];
  const hasCardValue = card ? [...allCardValue, ...allAddressKey.map((key) => card[key])].some((value) => value) : false;
  const hasAddressValue = detail.address ? Object.values(detail.address).some((value) => value) : false;
  const hasLicenseValue = detail.softwareLicense ? Object.values(detail.softwareLicense).some((value) => value) : false;
  // A server's linked SSH key is not shared: the link names an item the recipient cannot open.
  const hasServerValue = detail.server ? Boolean(detail.server.host || detail.server.port) : false;
  const database = detail.database;
  const hasDatabaseValue = database ? [database.engine, database.host, database.port, database.name, database.options].some((value) => value) : false;
  const hasIdentityValue = detail.identity ? Object.values(detail.identity).some((value) => value) : false;
  const credential = detail.apiCredential;
  const hasCredentialValue = credential ? [credential.service, credential.keyId, credential.secret, credential.host, credential.expiry].some((value) => value) : false;
  return Boolean(
    detail.username ||
      detail.password ||
      detail.notes ||
      detail.urls.length > 0 ||
      detail.fields.some((field) => field.value) ||
      hasCardValue ||
      hasIdentityValue ||
      hasCredentialValue ||
      hasAddressValue ||
      hasLicenseValue ||
      hasServerValue ||
      hasDatabaseValue,
  );
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
