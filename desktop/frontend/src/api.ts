import { Browser } from "@wailsio/runtime";
import { ApprovalTouchIDService, TouchIDPromptService } from "../bindings/github.com/tryhisar/hisar/desktop";
import { Service as AgentService } from "../bindings/github.com/tryhisar/hisar/desktop/agent";
import type { ApprovalEvent, Status as SSHAgentStatus } from "../bindings/github.com/tryhisar/hisar/desktop/agent";
import { BrowserBridge, CommandLineService, InviteLinkService, LoginItemService, Service, UpdateService } from "../bindings/github.com/tryhisar/hisar/desktop/vault";
import type { BrowserInstallation } from "../bindings/github.com/tryhisar/hisar/desktop/browser";
import type { Installation as CommandLineInstallation } from "../bindings/github.com/tryhisar/hisar/desktop/commandline";
import type {
  AccountDeletionPreview as BoundAccountDeletionPreview,
  BrowserPairingRequest,
  CredentialChangeResult,
  DeletionVault,
  BrowserStatus as BoundBrowserStatus,
  CommandLineStatus as BoundCommandLineStatus,
  Device,
  PairedBrowser,
  Person,
  ImportDuplicate,
  ImportFormat,
  ImportPreview as BoundImportPreview,
  ImportProgress as BoundImportProgress,
  ImportSkippedEntry,
  ItemDetail as BoundItemDetail,
  ItemField,
  ItemPasskey,
  LoginItemStatus,
  OneTimeCode,
  PasswordConfig,
  ReceivedShareInvitation,
  SentShareInvitation,
  ShareAwaitingUnlock,
  ExportOptions,
  ExportRequest,
  ExportResult,
  RemovedVaultNotice,
  VaultActions as BoundVaultActions,
  VaultDeletionRequest,
  VaultDeletionResult,
  ShareInvitations as BoundShareInvitations,
  ShareLink as BoundShareLink,
  ShareLinkField,
  ShareLinkRequest,
  SignUpResult,
  Status,
  Theme,
  UpdateRequirement,
  UpdateStatus,
  VaultMember,
  VaultSummary,
  WaitingShareInvitation,
} from "../bindings/github.com/tryhisar/hisar/desktop/vault";

export type {
  ApprovalEvent,
  CredentialChangeResult,
  DeletionVault,
  Device,
  ExportOptions,
  ExportRequest,
  ExportResult,
  RemovedVaultNotice,
  VaultDeletionRequest,
  VaultDeletionResult,
  Person,
  ImportDuplicate,
  ImportFormat,
  ImportSkippedEntry,
  ItemField,
  ItemPasskey,
  LoginItemStatus,
  OneTimeCode,
  PasswordConfig,
  ReceivedShareInvitation,
  SentShareInvitation,
  ShareAwaitingUnlock,
  ShareLinkField,
  ShareLinkRequest,
  SignUpResult,
  SSHAgentStatus,
  Status,
  Theme,
  UpdateRequirement,
  UpdateStatus,
  VaultMember,
  VaultSummary,
  WaitingShareInvitation,
};

export type SSHKeyType = "ed25519" | "rsa";
export type VaultRole = "owner" | "editor" | "reader";

// ShareInvitations: waiting lists the shares this user accepted that the owner's app has not finished yet.
export interface ShareInvitations {
  received: ReceivedShareInvitation[];
  sent: SentShareInvitation[];
  waiting?: WaitingShareInvitation[];
  completionError: string;
}

function normalizeShareInvitations(invitations: BoundShareInvitations): ShareInvitations {
  return {
    received: invitations.received ?? [],
    sent: invitations.sent ?? [],
    waiting: invitations.waiting ?? [],
    completionError: invitations.completionError,
  };
}

export interface VaultActions extends Omit<BoundVaultActions, "membersLosingAccess" | "targetVaults"> {
  membersLosingAccess: VaultMember[];
  targetVaults: VaultSummary[];
}

function normalizeVaultActions(actions: BoundVaultActions): VaultActions {
  return { ...actions, membersLosingAccess: actions.membersLosingAccess ?? [], targetVaults: actions.targetVaults ?? [] };
}

export interface ShareLink extends Omit<BoundShareLink, "fieldLabels"> {
  fieldLabels: string[];
}

function normalizeShareLink(boundShareLink: BoundShareLink): ShareLink {
  return { ...boundShareLink, fieldLabels: boundShareLink.fieldLabels ?? [] };
}

// ItemDetail mirrors the Go type with nil slices normalized to empty arrays.
export interface ItemDetail extends Omit<BoundItemDetail, "urls" | "fields" | "passkeys"> {
  urls: string[];
  fields: ItemField[];
  passkeys: ItemPasskey[];
}

function normalizeItem(boundItem: BoundItemDetail): ItemDetail {
  return { ...boundItem, urls: boundItem.urls ?? [], fields: boundItem.fields ?? [], passkeys: boundItem.passkeys ?? [] };
}

export type { BrowserInstallation, BrowserPairingRequest, CommandLineInstallation, PairedBrowser };

export interface CommandLineStatus extends Omit<BoundCommandLineStatus, "allInstallation"> {
  allInstallation: CommandLineInstallation[];
}

function normalizeCommandLineStatus(boundStatus: BoundCommandLineStatus): CommandLineStatus {
  return { ...boundStatus, allInstallation: boundStatus.allInstallation ?? [] };
}

export interface BrowserStatus extends Omit<BoundBrowserStatus, "allInstallation" | "allPairedBrowser"> {
  allInstallation: BrowserInstallation[];
  allPairedBrowser: PairedBrowser[];
}

function normalizeBrowserStatus(boundStatus: BoundBrowserStatus): BrowserStatus {
  return { ...boundStatus, allInstallation: boundStatus.allInstallation ?? [], allPairedBrowser: boundStatus.allPairedBrowser ?? [] };
}

export interface ImportPreview extends Omit<BoundImportPreview, "allSkipped" | "allDuplicate"> {
  allSkipped: ImportSkippedEntry[];
  allDuplicate: ImportDuplicate[];
}

function normalizeImportPreview(boundPreview: BoundImportPreview): ImportPreview {
  return { ...boundPreview, allSkipped: boundPreview.allSkipped ?? [], allDuplicate: boundPreview.allDuplicate ?? [] };
}

export interface ImportProgress extends Omit<BoundImportProgress, "allFailed"> {
  allFailed: ImportSkippedEntry[];
}

function normalizeImportProgress(boundProgress: BoundImportProgress): ImportProgress {
  return { ...boundProgress, allFailed: boundProgress.allFailed ?? [] };
}

export interface AccountDeletionPreview extends Omit<BoundAccountDeletionPreview, "ownedVaults" | "memberVaults"> {
  ownedVaults: DeletionVault[];
  memberVaults: DeletionVault[];
}

function normalizeAccountDeletionPreview(boundPreview: BoundAccountDeletionPreview): AccountDeletionPreview {
  return { ...boundPreview, ownedVaults: boundPreview.ownedVaults ?? [], memberVaults: boundPreview.memberVaults ?? [] };
}

const wailsApi = {
  getStatus: (): Promise<Status> => Service.GetStatus(),
  setAutoLock: (seconds: number): Promise<Status> => Service.SetAutoLock(seconds),
  getTheme: (): Promise<Theme> => Service.GetTheme(),
  setTheme: (appearance: string, color: string): Promise<Theme> => Service.SetTheme(appearance, color),
  signIn: (server: string, email: string, secretKey: string, password: string): Promise<Status> =>
    Service.SignIn(server, email, secretKey, password),
  signUp: (invite: string, email: string, name: string, password: string): Promise<SignUpResult> =>
    Service.SignUp(invite, email, name, password),
  unlock: (password: string): Promise<Status> => Service.Unlock(password),
  lock: (): Promise<Status> => Service.Lock(),
  signOut: (): Promise<Status> => Service.SignOut(),
  reportActivity: (): Promise<void> => Service.ReportActivity(),
  listAllVault: async (): Promise<VaultSummary[]> => (await Service.ListAllVault()) ?? [],
  createVault: (name: string): Promise<VaultSummary> => Service.CreateVault(name),
  getVaultActions: async (vaultID: string): Promise<VaultActions> => normalizeVaultActions(await Service.GetVaultActions(vaultID)),
  renameVault: (vaultID: string, name: string): Promise<VaultSummary> => Service.RenameVault(vaultID, name),
  deleteVault: (vaultID: string, request: VaultDeletionRequest): Promise<VaultDeletionResult> => Service.DeleteVault(vaultID, request),
  leaveVault: (vaultID: string): Promise<void> => Service.LeaveVault(vaultID),
  listAllRemovedVault: async (): Promise<RemovedVaultNotice[]> => (await Service.ListAllRemovedVault()) ?? [],
  getExportOptions: (): Promise<ExportOptions> => Service.GetExportOptions(),
  exportAllData: (request: ExportRequest): Promise<ExportResult> => Service.ExportAllData(request),
  listAllItem: async (vaultID: string): Promise<ItemDetail[]> => ((await Service.ListAllItem(vaultID)) ?? []).map(normalizeItem),
  getItem: async (vaultID: string, itemID: string): Promise<ItemDetail> => normalizeItem(await Service.GetItem(vaultID, itemID)),
  saveItem: async (vaultID: string, detail: ItemDetail): Promise<ItemDetail> =>
    normalizeItem(await Service.SaveItem(vaultID, detail)),
  deleteItem: (vaultID: string, itemID: string): Promise<void> => Service.DeleteItem(vaultID, itemID),
  deletePasskey: async (vaultID: string, itemID: string, credentialID: string): Promise<ItemDetail> =>
    normalizeItem(await Service.DeletePasskey(vaultID, itemID, credentialID)),
  generatePassword: (config: PasswordConfig): Promise<string> => Service.GeneratePassword(config),
  generateSSHKey: async (vaultID: string, title: string, keyType: SSHKeyType): Promise<ItemDetail> =>
    normalizeItem(await Service.GenerateSSHKey(vaultID, title, keyType)),
  importSSHKey: async (vaultID: string, title: string, privateKeyText: string, passphrase: string): Promise<ItemDetail> =>
    normalizeItem(await Service.ImportSSHKey(vaultID, title, privateKeyText, passphrase)),
  listAllPerson: async (): Promise<Person[]> => (await Service.ListAllPerson()) ?? [],
  listAllVaultMember: async (vaultID: string): Promise<VaultMember[]> => (await Service.ListAllVaultMember(vaultID)) ?? [],
  shareVault: (vaultID: string, email: string, role: VaultRole): Promise<void> => Service.ShareVault(vaultID, email, role),
  changeVaultMemberRole: (vaultID: string, accountID: string, role: VaultRole): Promise<void> =>
    Service.ChangeVaultMemberRole(vaultID, accountID, role),
  removeVaultMember: (vaultID: string, accountID: string): Promise<void> => Service.RemoveVaultMember(vaultID, accountID),
  invitePerson: (email: string): Promise<void> => Service.InvitePerson(email),
  listAllShareInvitation: async (): Promise<ShareInvitations> => normalizeShareInvitations(await Service.ListAllShareInvitation()),
  acceptShareInvitation: (invitationID: string): Promise<void> => Service.AcceptShareInvitation(invitationID),
  declineShareInvitation: (invitationID: string): Promise<void> => Service.DeclineShareInvitation(invitationID),
  cancelShareInvitation: (invitationID: string): Promise<void> => Service.CancelShareInvitation(invitationID),
  listAllShareAwaitingUnlock: async (): Promise<ShareAwaitingUnlock[]> => (await Service.ListAllShareAwaitingUnlock()) ?? [],
  listAllShareLinkField: async (vaultID: string, itemID: string): Promise<ShareLinkField[]> => (await Service.ListAllShareLinkField(vaultID, itemID)) ?? [],
  createShareLink: async (request: ShareLinkRequest): Promise<ShareLink> => normalizeShareLink(await Service.CreateShareLink(request)),
  listAllShareLink: async (): Promise<ShareLink[]> => ((await Service.ListAllShareLink()) ?? []).map(normalizeShareLink),
  revokeShareLink: (shareLinkID: string): Promise<void> => Service.RevokeShareLink(shareLinkID),
  copyShareLink: (shareLinkID: string): Promise<void> => Service.CopyShareLink(shareLinkID),
  listAllDevice: async (): Promise<Device[]> => (await Service.ListAllDevice()) ?? [],
  revokeDevice: (deviceID: string): Promise<void> => Service.RevokeDevice(deviceID),
  getSSHAgentStatus: (): Promise<SSHAgentStatus> => AgentService.GetStatus(),
  configureSSHConfig: (): Promise<SSHAgentStatus> => AgentService.ConfigureSSHConfig(),
  removeSSHConfig: (): Promise<SSHAgentStatus> => AgentService.RemoveSSHConfig(),
  respondSSHApproval: (requestID: string, isAuthorized: boolean, isRemembered: boolean): Promise<void> =>
    AgentService.RespondSSHApproval(requestID, isAuthorized, isRemembered),
  listAllPendingSSHApproval: async (): Promise<ApprovalEvent[]> => (await AgentService.ListAllPendingSSHApproval()) ?? [],
  respondApprovalWithPassword: (requestID: string, password: string, isRemembered: boolean): Promise<void> =>
    AgentService.RespondApprovalWithPassword(requestID, password, isRemembered),
  setSSHApprovalRemembered: (requestID: string, isRemembered: boolean): Promise<void> => AgentService.SetSSHApprovalRemembered(requestID, isRemembered),
  setCompactSSHApproval: (isCompact: boolean): Promise<SSHAgentStatus> => AgentService.SetCompactSSHApproval(isCompact),
  setPhoneApproval: (isEnabled: boolean): Promise<SSHAgentStatus> => AgentService.SetPhoneApproval(isEnabled),
  setSigningRemember: (seconds: number): Promise<SSHAgentStatus> => AgentService.SetSigningRemember(seconds),
  fitSSHApprovalWindow: (isCompact: boolean, height: number): Promise<void> => AgentService.FitSSHApprovalWindow(isCompact, height),
  setApprovalTouchIDFrame: (x: number, y: number, width: number, height: number): Promise<void> =>
    ApprovalTouchIDService.SetApprovalPromptFrame(x, y, width, height),
  clearApprovalTouchIDFrame: (): Promise<void> => ApprovalTouchIDService.ClearApprovalPromptFrame(),
  cancelApprovalTouchID: (): Promise<void> => ApprovalTouchIDService.CancelApprovalPrompt(),
  getOneTimeCode: (vaultID: string, itemID: string): Promise<OneTimeCode> => Service.GetOneTimeCode(vaultID, itemID),
  copyField: (vaultID: string, itemID: string, fieldName: string): Promise<void> =>
    Service.CopyField(vaultID, itemID, fieldName),
  copyText: (text: string): Promise<void> => Service.CopyText(text),
  copySecretReference: (vaultID: string, itemID: string, fieldName: string): Promise<string> =>
    Service.CopySecretReference(vaultID, itemID, fieldName),
  openURL: (link: string): Promise<void> => Browser.OpenURL(link),
  unlockWithBiometric: (): Promise<Status> => Service.UnlockWithBiometric(),
  enableBiometricUnlock: (password: string): Promise<Status> => Service.EnableBiometricUnlock(password),
  enableNewAccountBiometricUnlock: (): Promise<Status> => Service.EnableNewAccountBiometricUnlock(),
  disableBiometricUnlock: (): Promise<Status> => Service.DisableBiometricUnlock(),
  setTouchIDPromptFrame: (x: number, y: number, width: number, height: number): Promise<void> =>
    TouchIDPromptService.SetPromptFrame(x, y, width, height),
  clearTouchIDPromptFrame: (): Promise<void> => TouchIDPromptService.ClearPromptFrame(),
  cancelTouchIDPrompt: (): Promise<void> => TouchIDPromptService.CancelPrompt(),
  // hasNativeTouchIDView is true when macOS draws the Touch ID glyph over the prompt slot.
  hasNativeTouchIDView: true,
  saveNewAccountEmergencyKit: (): Promise<string> => Service.SaveNewAccountEmergencyKit(),
  saveEmergencyKit: (password: string): Promise<string> => Service.SaveEmergencyKit(password),
  forgetNewAccountSecretKey: (): Promise<void> => Service.ForgetNewAccountSecretKey(),
  changeMasterPassword: (currentPassword: string, newPassword: string): Promise<CredentialChangeResult> =>
    Service.ChangeMasterPassword(currentPassword, newPassword),
  replaceSecretKey: (password: string): Promise<CredentialChangeResult> => Service.ReplaceSecretKey(password),
  previewAccountDeletion: async (): Promise<AccountDeletionPreview> => normalizeAccountDeletionPreview(await Service.PreviewAccountDeletion()),
  deleteAccount: (password: string, email: string): Promise<Status> => Service.DeleteAccount(password, email),
  dismissSignedOutReason: (): Promise<Status> => Service.DismissSignedOutReason(),
  getBrowserStatus: async (): Promise<BrowserStatus> => normalizeBrowserStatus(await BrowserBridge.GetBrowserStatus()),
  removePairedBrowser: async (instanceID: string): Promise<BrowserStatus> =>
    normalizeBrowserStatus(await BrowserBridge.RemovePairedBrowser(instanceID)),
  setExactHostMatch: async (isExactHostMatch: boolean): Promise<BrowserStatus> =>
    normalizeBrowserStatus(await BrowserBridge.SetExactHostMatch(isExactHostMatch)),
  cancelBrowserPairing: (): Promise<void> => BrowserBridge.CancelBrowserPairing(),
  checkMinimumVersion: (): Promise<Status> => Service.CheckMinimumVersion(),
  takePendingInvite: (): Promise<string> => InviteLinkService.TakePendingInvite(),
  getUpdateStatus: (): Promise<UpdateStatus> => UpdateService.GetUpdateStatus(),
  checkForUpdates: (): Promise<UpdateStatus> => UpdateService.CheckForUpdates(),
  setAutomaticUpdateCheck: (isEnabled: boolean): Promise<UpdateStatus> => UpdateService.SetAutomaticUpdateCheck(isEnabled),
  getLoginItemStatus: (): Promise<LoginItemStatus> => LoginItemService.GetLoginItemStatus(),
  setOpenAtLogin: (isEnabled: boolean): Promise<LoginItemStatus> => LoginItemService.SetOpenAtLogin(isEnabled),
  openLoginItemSettings: (): Promise<void> => LoginItemService.OpenLoginItemSettings(),
  getCommandLineStatus: async (): Promise<CommandLineStatus> => normalizeCommandLineStatus(await CommandLineService.GetCommandLineStatus()),
  installCommandLineTool: async (): Promise<CommandLineStatus> => normalizeCommandLineStatus(await CommandLineService.InstallCommandLineTool()),
  uninstallCommandLineTool: async (): Promise<CommandLineStatus> => normalizeCommandLineStatus(await CommandLineService.UninstallCommandLineTool()),
  setCommandLineIntegration: async (isEnabled: boolean): Promise<CommandLineStatus> =>
    normalizeCommandLineStatus(await CommandLineService.SetCommandLineIntegration(isEnabled)),
  listAllAgentVaultID: async (): Promise<string[]> => (await Service.ListAllAgentVaultID()) ?? [],
  setVaultAvailableToAgents: async (vaultID: string, isAvailable: boolean): Promise<string[]> =>
    (await Service.SetVaultAvailableToAgents(vaultID, isAvailable)) ?? [],
  listAllImportFormat: async (): Promise<ImportFormat[]> => (await Service.ListAllImportFormat()) ?? [],
  chooseImportFile: async (vaultID: string, format: string): Promise<ImportPreview> =>
    normalizeImportPreview(await Service.ChooseImportFile(vaultID, format)),
  previewImport: async (vaultID: string): Promise<ImportPreview> => normalizeImportPreview(await Service.PreviewImport(vaultID)),
  startImport: async (vaultID: string, isSkippingDuplicate: boolean): Promise<ImportProgress> =>
    normalizeImportProgress(await Service.StartImport(vaultID, isSkippingDuplicate)),
  continueImport: async (): Promise<ImportProgress> => normalizeImportProgress(await Service.ContinueImport()),
  cancelImport: (): Promise<void> => Service.CancelImport(),
  deleteImportFile: (): Promise<void> => Service.DeleteImportFile(),
};

// main.ts swaps in the preview backend before mounting when VITE_PREVIEW is set.
export const api: typeof wailsApi = { ...wailsApi };

export function getErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  if (typeof error === "string" && error) {
    return error;
  }
  return "Something went wrong";
}

export function createEmptyItem(): ItemDetail {
  return {
    id: "",
    revision: "",
    category: "",
    title: "",
    username: "",
    urls: [],
    password: "",
    notes: "",
    fields: [],
    publicKey: "",
    fingerprint: "",
    keyType: "",
    oneTimePassword: "",
    hasOneTimePassword: false,
    passkeys: [],
  };
}
