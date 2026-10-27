import { Browser } from "@wailsio/runtime";
import { ApprovalTouchIDService, SettingsWindowService, TouchIDPromptService } from "../bindings/github.com/tryhisar/hisar/desktop";
import { Service as AgentService } from "../bindings/github.com/tryhisar/hisar/desktop/agent";
import type { ApprovalEvent, Status as SSHAgentStatus } from "../bindings/github.com/tryhisar/hisar/desktop/agent";
import { BrowserBridge, InviteLinkService, Service, UpdateService } from "../bindings/github.com/tryhisar/hisar/core/app";
import type { BrowserInstallation } from "../bindings/github.com/tryhisar/hisar/core/browserprotocol";
import { Service as CommandLineService } from "../bindings/github.com/tryhisar/hisar/desktop/commandline";
import type {
  Installation as CommandLineInstallation,
  LauncherApp,
  Status as BoundCommandLineStatus,
} from "../bindings/github.com/tryhisar/hisar/desktop/commandline";
import { Service as HarnessService } from "../bindings/github.com/tryhisar/hisar/desktop/harness";
import type { Agent as BoundCodingAgent } from "../bindings/github.com/tryhisar/hisar/desktop/harness";
import { Service as LoginItemService } from "../bindings/github.com/tryhisar/hisar/desktop/loginitem";
import type { Status as LoginItemStatus } from "../bindings/github.com/tryhisar/hisar/desktop/loginitem";
import type {
  AccountDeletionPreview as BoundAccountDeletionPreview,
  AgentGrant,
  AgentHost,
  AgentSession as BoundAgentSession,
  AgentSignIn,
  RememberedAgentSecret,
  BrowserPairingRequest,
  CardDescription,
  CredentialChangeResult,
  DeletionOrganization,
  DeletionVault,
  BrowserStatus as BoundBrowserStatus,
  Device,
  PairedBrowser,
  Person,
  ImportDuplicate,
  ImportFormat,
  ImportPreview as BoundImportPreview,
  ImportProgress as BoundImportProgress,
  ImportSkippedEntry,
  InviteLink,
  ItemAPICredential,
  ItemAddress,
  ItemAndroidApp,
  ItemCard,
  ItemDetail as BoundItemDetail,
  ItemIcon,
  ItemIdentity,
  ItemLocation,
  ItemField,
  ItemPasskey,
  ItemServer,
  ItemDatabase,
  ItemSoftwareLicense,
  OneTimeCode,
  Organization as BoundOrganization,
  OrganizationInvitation,
  OrganizationMember,
  Organizations as BoundOrganizations,
  OrganizationSummary,
  PendingRecoverySummary,
  PasswordConfig,
  ReceivedShareInvitation,
  SentOrganizationInvitation,
  SentShareInvitation,
  ShareAwaitingUnlock,
  ExportOptions,
  ExportRequest,
  ExportResult,
  FavoriteItem as BoundFavoriteItem,
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
  Team as BoundTeam,
  TeamMember,
  TeamVault,
  Theme,
  UpdateRequirement,
  UpdateStatus,
  VaultMember,
  VaultSummary,
  WaitingShareInvitation,
  WaitingVaultKey,
  WaitingVaultKeys as BoundWaitingVaultKeys,
} from "../bindings/github.com/tryhisar/hisar/core/app";

export type {
  AgentGrant,
  AgentHost,
  AgentSignIn,
  ApprovalEvent,
  RememberedAgentSecret,
  CardDescription,
  CredentialChangeResult,
  DeletionOrganization,
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
  InviteLink,
  ItemAPICredential,
  ItemAddress,
  ItemAndroidApp,
  ItemCard,
  ItemField,
  ItemIcon,
  ItemIdentity,
  ItemLocation,
  ItemPasskey,
  ItemServer,
  ItemDatabase,
  ItemSoftwareLicense,
  LoginItemStatus,
  OneTimeCode,
  OrganizationInvitation,
  OrganizationMember,
  OrganizationSummary,
  PasswordConfig,
  ReceivedShareInvitation,
  PendingRecoverySummary,
  SentOrganizationInvitation,
  SentShareInvitation,
  ShareAwaitingUnlock,
  ShareLinkField,
  ShareLinkRequest,
  SignUpResult,
  SSHAgentStatus,
  Status,
  TeamMember,
  TeamVault,
  Theme,
  UpdateRequirement,
  UpdateStatus,
  VaultMember,
  VaultSummary,
  WaitingShareInvitation,
  WaitingVaultKey,
};

export type SSHKeyType = "ed25519" | "rsa";
export type VaultRole = "owner" | "editor" | "reader";
export type OrganizationRole = "admin" | "member";
// MainWindowAction is what a Settings action leads to in the main window, which owns the vaults, the sharing panel and
// pending recovery.
export type MainWindowAction = "import" | "sharing" | "recovery" | "commit-signing";

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

export interface AgentSession extends Omit<BoundAgentSession, "allRememberedSecret"> {
  allRememberedSecret: RememberedAgentSecret[];
}

function normalizeAgentSessions(boundSessions: BoundAgentSession[] | null): AgentSession[] {
  return (boundSessions ?? []).map((session) => ({ ...session, allRememberedSecret: session.allRememberedSecret ?? [] }));
}

// ItemDetail mirrors the Go type with nil slices normalized to empty arrays.
export interface ItemDetail extends Omit<BoundItemDetail, "urls" | "tags" | "fields" | "passkeys" | "androidApps"> {
  urls: string[];
  tags: string[];
  fields: ItemField[];
  passkeys: ItemPasskey[];
  androidApps: ItemAndroidApp[];
}

function normalizeItem(boundItem: BoundItemDetail): ItemDetail {
  return {
    ...boundItem,
    urls: boundItem.urls ?? [],
    tags: boundItem.tags ?? [],
    fields: boundItem.fields ?? [],
    passkeys: boundItem.passkeys ?? [],
    androidApps: boundItem.androidApps ?? [],
  };
}

// FavoriteItem is one of the user's starred items with the vault it is in; item has no secrets.
export interface FavoriteItem extends Omit<BoundFavoriteItem, "item"> {
  item: ItemDetail;
}

function normalizeFavoriteItem(boundFavorite: BoundFavoriteItem): FavoriteItem {
  return { ...boundFavorite, item: normalizeItem(boundFavorite.item) };
}

export type { BrowserInstallation, BrowserPairingRequest, CommandLineInstallation, LauncherApp, PairedBrowser };

export interface CommandLineStatus extends Omit<BoundCommandLineStatus, "allInstallation"> {
  allInstallation: CommandLineInstallation[];
}

function normalizeCommandLineStatus(boundStatus: BoundCommandLineStatus): CommandLineStatus {
  return { ...boundStatus, allInstallation: boundStatus.allInstallation ?? [] };
}

// The bound enum also has Go's zero value, which the service returns only with an error.
export type CodingAgentState = "not-installed" | "available" | "connected" | "conflict" | "unknown";

export interface CodingAgent extends Omit<BoundCodingAgent, "state"> {
  state: CodingAgentState;
}

function normalizeCodingAgent(boundAgent: BoundCodingAgent): CodingAgent {
  return { ...boundAgent, state: String(boundAgent.state) as CodingAgentState };
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
  return { ...boundPreview, groups: boundPreview.groups ?? [], allSkipped: boundPreview.allSkipped ?? [], allDuplicate: boundPreview.allDuplicate ?? [] };
}

export interface ImportProgress extends Omit<BoundImportProgress, "allFailed" | "allSkipped" | "allDuplicateOutcomes"> {
  allFailed: ImportSkippedEntry[];
  allSkipped: ImportSkippedEntry[];
  allDuplicateOutcomes: ImportSkippedEntry[];
}

function normalizeImportProgress(boundProgress: BoundImportProgress): ImportProgress {
  return { ...boundProgress, allFailed: boundProgress.allFailed ?? [], allSkipped: boundProgress.allSkipped ?? [], allDuplicateOutcomes: boundProgress.allDuplicateOutcomes ?? [] };
}

export interface AccountDeletionPreview extends Omit<BoundAccountDeletionPreview, "ownedVaults" | "memberVaults" | "organizations"> {
  ownedVaults: DeletionVault[];
  memberVaults: DeletionVault[];
  organizations: DeletionOrganization[];
}

function normalizeAccountDeletionPreview(boundPreview: BoundAccountDeletionPreview): AccountDeletionPreview {
  return {
    ...boundPreview,
    ownedVaults: boundPreview.ownedVaults ?? [],
    memberVaults: boundPreview.memberVaults ?? [],
    organizations: boundPreview.organizations ?? [],
  };
}

export interface Organizations {
  organizations: OrganizationSummary[];
  invitations: OrganizationInvitation[];
}

function normalizeOrganizations(boundOrganizations: BoundOrganizations): Organizations {
  return { organizations: boundOrganizations.organizations ?? [], invitations: boundOrganizations.invitations ?? [] };
}

export interface Team extends Omit<BoundTeam, "members" | "vaults"> {
  members: TeamMember[];
  vaults: TeamVault[];
}

function normalizeTeam(boundTeam: BoundTeam): Team {
  return { ...boundTeam, members: boundTeam.members ?? [], vaults: boundTeam.vaults ?? [] };
}

// Organization: invitations is empty unless isAdmin; a team vault without canOpen has no name on this device.
export interface Organization extends Omit<BoundOrganization, "members" | "teams" | "invitations"> {
  members: OrganizationMember[];
  teams: Team[];
  invitations: SentOrganizationInvitation[];
}

function normalizeOrganization(boundOrganization: BoundOrganization): Organization {
  return {
    ...boundOrganization,
    members: boundOrganization.members ?? [],
    teams: (boundOrganization.teams ?? []).map(normalizeTeam),
    invitations: boundOrganization.invitations ?? [],
  };
}

// WaitingVaultKeys: deliveryError and rotationError say why this app could not give someone a vault key or re-encrypt a vault it owns.
export interface WaitingVaultKeys extends Omit<BoundWaitingVaultKeys, "waiting"> {
  waiting: WaitingVaultKey[];
}

function normalizeWaitingVaultKeys(boundWaiting: BoundWaitingVaultKeys): WaitingVaultKeys {
  return { ...boundWaiting, waiting: boundWaiting.waiting ?? [] };
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
  signUpWithoutInvite: (server: string, email: string, name: string, password: string): Promise<SignUpResult> =>
    Service.SignUpWithoutInvite(server, email, name, password),
  unlock: (password: string): Promise<Status> => Service.Unlock(password),
  lock: (): Promise<Status> => Service.Lock(),
  signOut: (): Promise<Status> => Service.SignOut(),
  reportActivity: (): Promise<void> => Service.ReportActivity(),
  listAllVault: async (): Promise<VaultSummary[]> => (await Service.ListVaults()) ?? [],
  createVault: (name: string): Promise<VaultSummary> => Service.CreateVault(name),
  getVaultActions: async (vaultID: string): Promise<VaultActions> => normalizeVaultActions(await Service.GetVaultActions(vaultID)),
  renameVault: (vaultID: string, name: string): Promise<VaultSummary> => Service.RenameVault(vaultID, name),
  deleteVault: (vaultID: string, request: VaultDeletionRequest): Promise<VaultDeletionResult> => Service.DeleteVault(vaultID, request),
  leaveVault: (vaultID: string): Promise<void> => Service.LeaveVault(vaultID),
  listAllRemovedVault: async (): Promise<RemovedVaultNotice[]> => (await Service.ListRemovedVaults()) ?? [],
  getExportOptions: (): Promise<ExportOptions> => Service.GetExportOptions(),
  exportAllData: (request: ExportRequest): Promise<ExportResult> => Service.ExportData(request),
  listAllItem: async (vaultID: string): Promise<ItemDetail[]> => ((await Service.ListItems(vaultID)) ?? []).map(normalizeItem),
  getItem: async (vaultID: string, itemID: string): Promise<ItemDetail> => normalizeItem(await Service.GetItem(vaultID, itemID)),
  saveItem: async (vaultID: string, detail: ItemDetail): Promise<ItemDetail> =>
    normalizeItem(await Service.SaveItem(vaultID, detail)),
  deleteItem: (vaultID: string, itemID: string): Promise<void> => Service.DeleteItem(vaultID, itemID),
  deleteUnreadableItem: (vaultID: string, itemID: string, revision: string): Promise<void> => Service.DeleteUnreadableItem(vaultID, itemID, revision),
  deleteUnreadableVault: (vaultID: string, revision: string): Promise<void> => Service.DeleteUnreadableVault(vaultID, revision),
  repairVaultName: (vaultID: string, revision: string, name: string): Promise<VaultSummary> => Service.RepairVaultName(vaultID, revision, name),
  moveItem: async (vaultID: string, itemID: string, targetVaultID: string): Promise<ItemDetail> =>
    normalizeItem(await Service.MoveItem(vaultID, itemID, targetVaultID)),
  duplicateItem: async (vaultID: string, itemID: string): Promise<ItemDetail> => normalizeItem(await Service.DuplicateItem(vaultID, itemID)),
  // Images cross as base64: prepareItemIcon takes a file's bytes and returns the PNG an item's icon keeps.
  prepareItemIcon: async (imageBase64: string): Promise<string> => (await Service.PrepareItemIcon(imageBase64)) ?? "",
  // getWebsiteIcon returns a login website's icon as base64 PNG, or "" when there is none.
  getWebsiteIcon: async (url: string): Promise<string> => (await Service.GetWebsiteIcon(url)) ?? "",
  isWebsiteIconsEnabled: (): Promise<boolean> => Service.IsWebsiteIconsEnabled(),
  setWebsiteIconsEnabled: (isEnabled: boolean): Promise<void> => Service.SetWebsiteIconsEnabled(isEnabled),
  listAllFavoriteItem: async (): Promise<FavoriteItem[]> => ((await Service.ListFavoriteItems()) ?? []).map(normalizeFavoriteItem),
  setItemFavorite: (vaultID: string, itemID: string, isFavorite: boolean): Promise<void> => Service.SetItemFavorite(vaultID, itemID, isFavorite),
  describeCard: (number: string, expiry: string): Promise<CardDescription> => Service.DescribeCard(number, expiry),
  convertToSecureNote: async (vaultID: string, itemID: string): Promise<ItemDetail> =>
    normalizeItem(await Service.ConvertToSecureNote(vaultID, itemID)),
  deletePasskey: async (vaultID: string, itemID: string, credentialID: string): Promise<ItemDetail> =>
    normalizeItem(await Service.DeletePasskey(vaultID, itemID, credentialID)),
  generatePassword: (config: PasswordConfig): Promise<string> => Service.GeneratePassword(config),
  generateSSHKey: async (vaultID: string, title: string, keyType: SSHKeyType): Promise<ItemDetail> =>
    normalizeItem(await Service.GenerateSSHKey(vaultID, title, keyType)),
  importSSHKey: async (vaultID: string, title: string, privateKeyText: string, passphrase: string): Promise<ItemDetail> =>
    normalizeItem(await Service.ImportSSHKey(vaultID, title, privateKeyText, passphrase)),
  listAllPerson: async (): Promise<Person[]> => (await Service.ListPeople()) ?? [],
  listAllVaultMember: async (vaultID: string): Promise<VaultMember[]> => (await Service.ListVaultMembers(vaultID)) ?? [],
  shareVault: (vaultID: string, email: string, role: VaultRole): Promise<void> => Service.ShareVault(vaultID, email, role),
  changeVaultMemberRole: (vaultID: string, accountID: string, role: VaultRole): Promise<void> =>
    Service.ChangeVaultMemberRole(vaultID, accountID, role),
  removeVaultMember: (vaultID: string, accountID: string): Promise<void> => Service.RemoveVaultMember(vaultID, accountID),
  invitePerson: (email: string): Promise<void> => Service.InvitePerson(email),
  listAllShareInvitation: async (): Promise<ShareInvitations> => normalizeShareInvitations(await Service.ListShareInvitations()),
  acceptShareInvitation: (invitationID: string): Promise<void> => Service.AcceptShareInvitation(invitationID),
  declineShareInvitation: (invitationID: string): Promise<void> => Service.DeclineShareInvitation(invitationID),
  cancelShareInvitation: (invitationID: string): Promise<void> => Service.CancelShareInvitation(invitationID),
  confirmShareRecipient: (invitationID: string, reviewToken: string): Promise<void> => Service.ConfirmShareRecipient(invitationID, reviewToken),
  listAllOrganization: async (): Promise<Organizations> => normalizeOrganizations(await Service.ListOrganizations()),
  getOrganization: async (organizationID: string): Promise<Organization> => normalizeOrganization(await Service.GetOrganization(organizationID)),
  createOrganization: (name: string): Promise<OrganizationSummary> => Service.CreateOrganization(name),
  renameOrganization: (organizationID: string, name: string): Promise<void> => Service.RenameOrganization(organizationID, name),
  deleteOrganization: (organizationID: string): Promise<void> => Service.DeleteOrganization(organizationID),
  leaveOrganization: (organizationID: string): Promise<void> => Service.LeaveOrganization(organizationID),
  inviteToOrganization: (organizationID: string, email: string, role: OrganizationRole): Promise<void> =>
    Service.InviteToOrganization(organizationID, email, role),
  cancelOrganizationInvitation: (organizationID: string, invitationID: string): Promise<void> =>
    Service.CancelOrganizationInvitation(organizationID, invitationID),
  acceptOrganizationInvitation: (invitationID: string): Promise<void> => Service.AcceptOrganizationInvitation(invitationID),
  declineOrganizationInvitation: (invitationID: string): Promise<void> => Service.DeclineOrganizationInvitation(invitationID),
  setOrganizationMemberRole: (organizationID: string, accountID: string, role: OrganizationRole): Promise<void> =>
    Service.SetOrganizationMemberRole(organizationID, accountID, role),
  removeOrganizationMember: (organizationID: string, accountID: string): Promise<void> => Service.RemoveOrganizationMember(organizationID, accountID),
  createTeam: async (organizationID: string, name: string): Promise<Team> => normalizeTeam(await Service.CreateTeam(organizationID, name)),
  renameTeam: (organizationID: string, teamID: string, name: string): Promise<void> => Service.RenameTeam(organizationID, teamID, name),
  deleteTeam: (organizationID: string, teamID: string): Promise<void> => Service.DeleteTeam(organizationID, teamID),
  addTeamMember: (organizationID: string, teamID: string, accountID: string): Promise<void> => Service.AddTeamMember(organizationID, teamID, accountID),
  removeTeamMember: (organizationID: string, teamID: string, accountID: string): Promise<void> =>
    Service.RemoveTeamMember(organizationID, teamID, accountID),
  giveVaultToTeam: (organizationID: string, teamID: string, vaultID: string, role: VaultRole): Promise<void> =>
    Service.GiveVaultToTeam(organizationID, teamID, vaultID, role),
  takeVaultFromTeam: (organizationID: string, teamID: string, vaultID: string): Promise<void> =>
    Service.TakeVaultFromTeam(organizationID, teamID, vaultID),
  createOrganizationVault: (organizationID: string, teamID: string, name: string): Promise<VaultSummary> =>
    Service.CreateOrganizationVault(organizationID, teamID, name),
  listAllWaitingVaultKey: async (): Promise<WaitingVaultKeys> => normalizeWaitingVaultKeys(await Service.ListWaitingVaultKeys()),
  listAllShareAwaitingUnlock: async (): Promise<ShareAwaitingUnlock[]> => (await Service.ListSharesAwaitingUnlock()) ?? [],
  listAllShareLinkField: async (vaultID: string, itemID: string): Promise<ShareLinkField[]> => (await Service.ListShareLinkFields(vaultID, itemID)) ?? [],
  createShareLink: async (request: ShareLinkRequest): Promise<ShareLink> => normalizeShareLink(await Service.CreateShareLink(request)),
  listAllShareLink: async (): Promise<ShareLink[]> => ((await Service.ListShareLinks()) ?? []).map(normalizeShareLink),
  revokeShareLink: (shareLinkID: string): Promise<void> => Service.RevokeShareLink(shareLinkID),
  copyShareLink: (shareLinkID: string): Promise<void> => Service.CopyShareLink(shareLinkID),
  listAllDevice: async (): Promise<Device[]> => (await Service.ListDevices()) ?? [],
  revokeDevice: (deviceID: string): Promise<void> => Service.RevokeDevice(deviceID),
  getSSHAgentStatus: (): Promise<SSHAgentStatus> => AgentService.GetStatus(),
  configureSSHConfig: (): Promise<SSHAgentStatus> => AgentService.ConfigureSSHConfig(),
  removeSSHConfig: (): Promise<SSHAgentStatus> => AgentService.RemoveSSHConfig(),
  respondSSHApproval: (requestID: string, isAuthorized: boolean, isRemembered: boolean): Promise<void> =>
    AgentService.RespondSSHApproval(requestID, isAuthorized, isRemembered),
  listAllPendingSSHApproval: async (): Promise<ApprovalEvent[]> => (await AgentService.ListPendingSSHApprovals()) ?? [],
  respondApprovalWithPassword: (requestID: string, password: string, isRemembered: boolean): Promise<void> =>
    AgentService.RespondApprovalWithPassword(requestID, password, isRemembered),
  setSSHApprovalRemembered: (requestID: string, isRemembered: boolean): Promise<void> => AgentService.SetSSHApprovalRemembered(requestID, isRemembered),
  setSSHApprovalGrantDuration: (requestID: string, duration: string): Promise<void> => AgentService.SetSSHApprovalGrantDuration(requestID, duration),
  setSSHApprovalLogin: (requestID: string, loginIndex: number): Promise<void> => AgentService.SetSSHApprovalLogin(requestID, loginIndex),
  setCompactSSHApproval: (isCompact: boolean): Promise<SSHAgentStatus> => AgentService.SetCompactSSHApproval(isCompact),
  setPhoneApproval: (isEnabled: boolean): Promise<SSHAgentStatus> => AgentService.SetPhoneApproval(isEnabled),
  setSigningRemember: (seconds: number): Promise<SSHAgentStatus> => AgentService.SetSigningRemember(seconds),
  setSSHRemember: (seconds: number): Promise<SSHAgentStatus> => AgentService.SetSSHRemember(seconds),
  fitSSHApprovalWindow: (isCompact: boolean, height: number): Promise<void> => AgentService.FitSSHApprovalWindow(isCompact, height),
  setApprovalTouchIDFrame: (x: number, y: number, width: number, height: number): Promise<void> =>
    ApprovalTouchIDService.SetApprovalPromptFrame(x, y, width, height),
  clearApprovalTouchIDFrame: (): Promise<void> => ApprovalTouchIDService.ClearApprovalPromptFrame(),
  cancelApprovalTouchID: (): Promise<void> => ApprovalTouchIDService.CancelApprovalPrompt(),
  // showSettingsWindow opens Settings in its own window and reports false where Settings is an overlay instead (Linux).
  // While Hisar is locked the window opens after the unlock.
  showSettingsWindow: (sectionID: string): Promise<boolean> => SettingsWindowService.Show(sectionID),
  hideSettingsWindow: (): Promise<void> => SettingsWindowService.Hide(),
  // takeSettingsWindowRequest returns the section the Settings window was asked to open at ("" for its last), once, or null.
  takeSettingsWindowRequest: async (): Promise<string | null> => {
    const [sectionID, isRequested] = await SettingsWindowService.TakeOpenRequest();
    return isRequested ? sectionID : null;
  },
  // openInMainWindow closes the Settings window and has the main window open what a Settings action leads to.
  openInMainWindow: (action: MainWindowAction): Promise<void> => SettingsWindowService.OpenInMainWindow(action),
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
  listPendingRecovery: async (): Promise<PendingRecoverySummary[]> => (await Service.ListPendingRecovery()) ?? [],
  resumePendingRecovery: (operationID: string, password: string, secretKey = "", priorSecretKey = ""): Promise<SignUpResult> => Service.ResumePendingRecoveryWithSecretKey(operationID, password, secretKey, priorSecretKey),
  savePendingRecoveryEmergencyKit: (operationID: string, password: string): Promise<string> => Service.SavePendingRecoveryEmergencyKit(operationID, password),
  acknowledgePendingRecovery: (operationID: string, password: string): Promise<void> => Service.AcknowledgePendingRecovery(operationID, password),
  discardPreparedRecovery: (operationID: string, password: string): Promise<void> => Service.DiscardPreparedRecovery(operationID, password),
  changeMasterPassword: (currentPassword: string, newPassword: string): Promise<CredentialChangeResult> =>
    Service.ChangeMasterPassword(currentPassword, newPassword),
  replaceSecretKey: (password: string): Promise<CredentialChangeResult> => Service.ReplaceSecretKey(password),
  previewAccountDeletion: async (): Promise<AccountDeletionPreview> => normalizeAccountDeletionPreview(await Service.PreviewAccountDeletion()),
  deleteAccount: (password: string, email: string, planVersion: string): Promise<Status> => Service.DeleteAccountWithPlan(password, email, planVersion),
  dismissSignedOutReason: (): Promise<Status> => Service.DismissSignedOutReason(),
  getBrowserStatus: async (): Promise<BrowserStatus> => normalizeBrowserStatus(await BrowserBridge.GetBrowserStatus()),
  removePairedBrowser: async (instanceID: string): Promise<BrowserStatus> =>
    normalizeBrowserStatus(await BrowserBridge.RemovePairedBrowser(instanceID)),
  setExactHostMatch: async (isExactHostMatch: boolean): Promise<BrowserStatus> =>
    normalizeBrowserStatus(await BrowserBridge.SetExactHostMatch(isExactHostMatch)),
  cancelBrowserPairing: (): Promise<void> => BrowserBridge.CancelBrowserPairing(),
  checkMinimumVersion: (): Promise<Status> => Service.CheckMinimumVersion(),
  takePendingInvite: (): Promise<InviteLink | null> => InviteLinkService.TakePendingInvite(),
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
  listAllLauncherApp: async (): Promise<LauncherApp[]> => (await CommandLineService.ListLauncherApps()) ?? [],
  revokeLauncherApp: async (appPath: string): Promise<LauncherApp[]> => (await CommandLineService.RevokeLauncherApp(appPath)) ?? [],
  listAllCodingAgent: async (): Promise<CodingAgent[]> => ((await HarnessService.ListAgents()) ?? []).map(normalizeCodingAgent),
  connectCodingAgent: async (id: string): Promise<CodingAgent> => normalizeCodingAgent(await HarnessService.Connect(id)),
  disconnectCodingAgent: async (id: string): Promise<CodingAgent> => normalizeCodingAgent(await HarnessService.Disconnect(id)),
  listAllAgentVaultID: async (): Promise<string[]> => (await Service.ListAgentVaultIDs()) ?? [],
  setVaultAvailableToAgents: async (vaultID: string, isAvailable: boolean): Promise<string[]> =>
    (await Service.SetVaultAvailableToAgents(vaultID, isAvailable)) ?? [],
  listAllAgentSession: async (): Promise<AgentSession[]> => normalizeAgentSessions(await Service.ListAgentSessions()),
  revokeAgentSession: async (pid: number, startTime: number): Promise<AgentSession[]> =>
    normalizeAgentSessions(await Service.RevokeAgentSession(pid, startTime)),
  listAllAgentGrant: async (): Promise<AgentGrant[]> => (await Service.ListAgentGrants()) ?? [],
  revokeAgentGrant: async (id: string): Promise<AgentGrant[]> => (await Service.RevokeAgentGrant(id)) ?? [],
  getAgentSignIn: (): Promise<AgentSignIn> => Service.GetAgentSignIn(),
  setAgentSignInEnabled: (isEnabled: boolean): Promise<AgentSignIn> => Service.SetAgentSignInEnabled(isEnabled),
  listAllAgentHost: async (): Promise<AgentHost[]> => (await Service.ListAgentHosts()) ?? [],
  forgetAgentHost: async (path: string, teamID: string): Promise<AgentHost[]> => (await Service.ForgetAgentHost(path, teamID)) ?? [],
  listAllImportFormat: async (): Promise<ImportFormat[]> => (await Service.ListImportFormats()) ?? [],
  chooseImportFile: async (vaultID: string, format: string): Promise<ImportPreview> =>
    normalizeImportPreview(await Service.ChooseImportFile(vaultID, format)),
  openImportFile: async (passphrase: string): Promise<ImportPreview> => normalizeImportPreview(await Service.OpenImportFile(passphrase)),
  previewImport: async (vaultID: string): Promise<ImportPreview> => normalizeImportPreview(await Service.PreviewImport(vaultID)),
  mapImportGroup: async (groupID: string, vaultID: string): Promise<ImportPreview> => normalizeImportPreview(await Service.MapImportGroup(groupID, vaultID)),
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

export function createEmptyAddress(): ItemAddress {
  return { street: "", city: "", region: "", postalCode: "", country: "" };
}

export function createEmptyCard(): ItemCard {
  return {
    cardholder: "",
    type: "",
    number: "",
    expiry: "",
    securityCode: "",
    pin: "",
    ...createEmptyAddress(),
    billingAddress: "",
    brand: "",
    last4: "",
    numberWarning: "",
    expiryWarning: "",
  };
}

export function createEmptyIdentity(): ItemIdentity {
  return {
    firstName: "",
    middleName: "",
    lastName: "",
    birthDate: "",
    email: "",
    phone: "",
    ...createEmptyAddress(),
    address: "",
    passportNumber: "",
    driversLicenseNumber: "",
    nationalIdNumber: "",
  };
}

export function createEmptyAPICredential(): ItemAPICredential {
  return { service: "", keyId: "", secret: "", host: "", expiry: "", expiryWarning: "" };
}

export function createEmptyServer(): ItemServer {
  return { host: "", port: "", sshKeyId: "", sshKeyTitle: "", sshKeyFingerprint: "", portWarning: "" };
}

export function createEmptyDatabase(): ItemDatabase {
  return { engine: "postgresql", engineName: "PostgreSQL", host: "", port: "", name: "", options: "", connectionString: "", portWarning: "" };
}

export function createEmptySoftwareLicense(): ItemSoftwareLicense {
  return { product: "", version: "", licenseKey: "", licensedTo: "", email: "", purchaseDate: "", downloadUrl: "" };
}

export function createEmptyItem(): ItemDetail {
  return {
    id: "",
    revision: "",
    category: "",
    title: "",
    username: "",
    urls: [],
    tags: [],
    password: "",
    notes: "",
    fields: [],
    publicKey: "",
    fingerprint: "",
    keyType: "",
    oneTimePassword: "",
    hasOneTimePassword: false,
    passkeys: [],
    androidApps: [],
    canConvertToSecureNote: false,
    isFavorite: false,
  };
}
