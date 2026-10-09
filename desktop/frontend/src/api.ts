import { Browser } from "@wailsio/runtime";
import { TouchIDPromptService } from "../bindings/github.com/tryhisar/hisar/desktop";
import { Service as AgentService } from "../bindings/github.com/tryhisar/hisar/desktop/agent";
import type { ApprovalEvent, Status as SSHAgentStatus } from "../bindings/github.com/tryhisar/hisar/desktop/agent";
import { BrowserBridge, Service } from "../bindings/github.com/tryhisar/hisar/desktop/vault";
import type { BrowserInstallation } from "../bindings/github.com/tryhisar/hisar/desktop/browser";
import type {
  BrowserPairingRequest,
  BrowserStatus as BoundBrowserStatus,
  Device,
  PairedBrowser,
  FamilyMember,
  ItemDetail as BoundItemDetail,
  ItemField,
  PasswordConfig,
  PendingRecovery,
  RecoverAccountResult,
  RecoveryStatus,
  SignUpResult,
  Status,
  VaultMember,
  VaultSummary,
} from "../bindings/github.com/tryhisar/hisar/desktop/vault";

export type {
  ApprovalEvent,
  Device,
  FamilyMember,
  ItemField,
  PasswordConfig,
  PendingRecovery,
  RecoverAccountResult,
  RecoveryStatus,
  SignUpResult,
  SSHAgentStatus,
  Status,
  VaultMember,
  VaultSummary,
};

export type SSHKeyType = "ed25519" | "rsa";
export type VaultRole = "owner" | "editor" | "reader";
export type FamilyRole = "admin" | "member";

// ItemDetail mirrors the Go type with nil slices normalized to empty arrays.
export interface ItemDetail extends Omit<BoundItemDetail, "urls" | "fields"> {
  urls: string[];
  fields: ItemField[];
}

function normalizeItem(boundItem: BoundItemDetail): ItemDetail {
  return { ...boundItem, urls: boundItem.urls ?? [], fields: boundItem.fields ?? [] };
}

export type { BrowserInstallation, BrowserPairingRequest, PairedBrowser };

export interface BrowserStatus extends Omit<BoundBrowserStatus, "allInstallation" | "allPairedBrowser"> {
  allInstallation: BrowserInstallation[];
  allPairedBrowser: PairedBrowser[];
}

function normalizeBrowserStatus(boundStatus: BoundBrowserStatus): BrowserStatus {
  return { ...boundStatus, allInstallation: boundStatus.allInstallation ?? [], allPairedBrowser: boundStatus.allPairedBrowser ?? [] };
}

const wailsApi = {
  getStatus: (): Promise<Status> => Service.GetStatus(),
  signIn: (server: string, email: string, secretKey: string, password: string): Promise<Status> =>
    Service.SignIn(server, email, secretKey, password),
  signUp: (inviteLink: string, email: string, name: string, password: string): Promise<SignUpResult> =>
    Service.SignUp(inviteLink, email, name, password),
  unlock: (password: string): Promise<Status> => Service.Unlock(password),
  lock: (): Promise<Status> => Service.Lock(),
  signOut: (): Promise<Status> => Service.SignOut(),
  reportActivity: (): Promise<void> => Service.ReportActivity(),
  listAllVault: async (): Promise<VaultSummary[]> => (await Service.ListAllVault()) ?? [],
  createVault: (name: string): Promise<VaultSummary> => Service.CreateVault(name),
  listAllItem: async (vaultID: string): Promise<ItemDetail[]> => ((await Service.ListAllItem(vaultID)) ?? []).map(normalizeItem),
  getItem: async (vaultID: string, itemID: string): Promise<ItemDetail> => normalizeItem(await Service.GetItem(vaultID, itemID)),
  saveItem: async (vaultID: string, detail: ItemDetail): Promise<ItemDetail> =>
    normalizeItem(await Service.SaveItem(vaultID, detail)),
  deleteItem: (vaultID: string, itemID: string): Promise<void> => Service.DeleteItem(vaultID, itemID),
  generatePassword: (config: PasswordConfig): Promise<string> => Service.GeneratePassword(config),
  generateSSHKey: async (vaultID: string, title: string, keyType: SSHKeyType): Promise<ItemDetail> =>
    normalizeItem(await Service.GenerateSSHKey(vaultID, title, keyType)),
  importSSHKey: async (vaultID: string, title: string, privateKeyText: string, passphrase: string): Promise<ItemDetail> =>
    normalizeItem(await Service.ImportSSHKey(vaultID, title, privateKeyText, passphrase)),
  listAllMember: async (): Promise<FamilyMember[]> => (await Service.ListAllMember()) ?? [],
  listAllVaultMember: async (vaultID: string): Promise<VaultMember[]> => (await Service.ListAllVaultMember(vaultID)) ?? [],
  shareVault: (vaultID: string, email: string, role: VaultRole): Promise<void> => Service.ShareVault(vaultID, email, role),
  changeVaultMemberRole: (vaultID: string, accountID: string, role: VaultRole): Promise<void> =>
    Service.ChangeVaultMemberRole(vaultID, accountID, role),
  removeVaultMember: (vaultID: string, accountID: string): Promise<void> => Service.RemoveVaultMember(vaultID, accountID),
  inviteMember: (email: string, role: FamilyRole): Promise<string> => Service.InviteMember(email, role),
  listAllDevice: async (): Promise<Device[]> => (await Service.ListAllDevice()) ?? [],
  revokeDevice: (deviceID: string): Promise<void> => Service.RevokeDevice(deviceID),
  getSSHAgentStatus: (): Promise<SSHAgentStatus> => AgentService.GetStatus(),
  configureSSHConfig: (): Promise<SSHAgentStatus> => AgentService.ConfigureSSHConfig(),
  removeSSHConfig: (): Promise<SSHAgentStatus> => AgentService.RemoveSSHConfig(),
  respondSSHApproval: (requestID: string, isAllowed: boolean): Promise<void> => AgentService.RespondSSHApproval(requestID, isAllowed),
  copyField: (vaultID: string, itemID: string, fieldName: string): Promise<void> =>
    Service.CopyField(vaultID, itemID, fieldName),
  copyText: (text: string): Promise<void> => Service.CopyText(text),
  openURL: (link: string): Promise<void> => Browser.OpenURL(link),
  unlockWithBiometric: (): Promise<Status> => Service.UnlockWithBiometric(),
  enableBiometricUnlock: (password: string): Promise<Status> => Service.EnableBiometricUnlock(password),
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
  getBrowserStatus: async (): Promise<BrowserStatus> => normalizeBrowserStatus(await BrowserBridge.GetBrowserStatus()),
  removePairedBrowser: async (instanceID: string): Promise<BrowserStatus> =>
    normalizeBrowserStatus(await BrowserBridge.RemovePairedBrowser(instanceID)),
  setExactHostMatch: async (isExactHostMatch: boolean): Promise<BrowserStatus> =>
    normalizeBrowserStatus(await BrowserBridge.SetExactHostMatch(isExactHostMatch)),
  cancelBrowserPairing: (): Promise<void> => BrowserBridge.CancelBrowserPairing(),
  getRecoveryStatus: (): Promise<RecoveryStatus> => Service.GetRecoveryStatus(),
  setUpRecovery: async (): Promise<string[]> => (await Service.SetUpRecovery()) ?? [],
  enableRecoveryForAllVault: async (): Promise<string[]> => (await Service.EnableRecoveryForAllVault()) ?? [],
  enableVaultRecovery: (vaultID: string): Promise<VaultSummary> => Service.EnableVaultRecovery(vaultID),
  disableVaultRecovery: (vaultID: string): Promise<VaultSummary> => Service.DisableVaultRecovery(vaultID),
  startAccountRecovery: (accountID: string): Promise<string> => Service.StartAccountRecovery(accountID),
  listAllPendingRecovery: async (): Promise<PendingRecovery[]> => (await Service.ListAllPendingRecovery()) ?? [],
  completeAccountRecovery: async (accountID: string): Promise<string[]> => (await Service.CompleteAccountRecovery(accountID)) ?? [],
  recoverAccount: (recoveryLink: string, email: string, newPassword: string): Promise<RecoverAccountResult> =>
    Service.RecoverAccount(recoveryLink, email, newPassword),
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
  };
}
