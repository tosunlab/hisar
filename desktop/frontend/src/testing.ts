import { vi } from "vitest";
import type { ApprovalEvent, BrowserStatus, CommandLineStatus, Device, FamilyMember, ImportPreview, ImportProgress, ItemDetail, PendingRecovery, RecoveryStatus, SSHAgentStatus, Status, UpdateStatus, VaultMember, VaultSummary } from "./api";

export function createStatus(state: string): Status {
  return { state, server: "https://hisar.test", email: "alice@example.com", name: "Alice", biometricAvailable: false, biometricEnabled: false, biometricReady: false, isOffline: false };
}

export function createItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return {
    id: "item-1",
    revision: "rev-1",
    category: "",
    title: "GitHub",
    username: "alice",
    urls: ["https://github.com"],
    password: "",
    notes: "",
    fields: [],
    publicKey: "",
    fingerprint: "",
    keyType: "",
    oneTimePassword: "",
    hasOneTimePassword: false,
    passkeys: [],
    ...overrides,
  };
}

export function createSSHKeyItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "ssh-1",
    category: "ssh-key",
    title: "Laptop key",
    username: "",
    urls: [],
    publicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHisarTestKey Laptop key",
    fingerprint: "SHA256:hisarTestFingerprint",
    keyType: "ssh-ed25519",
    ...overrides,
  });
}

export function createSSHAgentStatus(overrides: Partial<SSHAgentStatus> = {}): SSHAgentStatus {
  return {
    socketPath: "/Users/alice/.hisar/agent.sock",
    isRunning: true,
    errorMessage: "",
    isSSHConfigured: false,
    configPath: "/Users/alice/.ssh/config",
    configSection: '# Added by Hisar: use the Hisar SSH agent\nHost *\n  IdentityAgent "~/.hisar/agent.sock"\n# End of Hisar section\n',
    isTouchIDApproval: true,
    backupPath: "",
    ...overrides,
  };
}

export function createApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return {
    requestID: "request-1",
    kind: "ssh-key",
    terminalName: "",
    isUnlockRequired: false,
    isPasswordUnlock: false,
    appName: "Terminal",
    appIconURL: "",
    clientName: "ssh",
    clientPath: "/usr/bin/ssh",
    keyTitle: "Laptop key",
    fingerprint: "SHA256:hisarTestFingerprint",
    destinationHost: "github.com",
    destinationFingerprint: "SHA256:hostFingerprint",
    isForwarded: false,
    isTouchIDApproval: true,
    timeoutAt: "2026-10-09T12:01:00Z",
    ...overrides,
  };
}

export function createCommandLineApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return createApprovalEvent({
    requestID: "command-line-1",
    kind: "command-line",
    clientName: "hisar",
    clientPath: "/Applications/Hisar.app/Contents/Helpers/hisar",
    keyTitle: "",
    fingerprint: "",
    destinationHost: "",
    destinationFingerprint: "",
    terminalName: "ttys003",
    ...overrides,
  });
}

export function createCommandLineStatus(overrides: Partial<CommandLineStatus> = {}): CommandLineStatus {
  return {
    isAvailable: true,
    bundledPath: "/Applications/Hisar.app/Contents/Helpers/hisar",
    installedPath: "",
    allInstallation: [],
    pathCommandPath: "",
    pathCommandTarget: "",
    isPathCommandFromThisApp: false,
    pathHint: "",
    isIntegrationEnabled: true,
    socketPath: "/Users/alice/.hisar/cli.sock",
    isServing: true,
    errorMessage: "",
    ...overrides,
  };
}

export function createFamilyMember(overrides: Partial<FamilyMember> = {}): FamilyMember {
  return {
    accountId: "ALICE1",
    name: "Alice",
    email: "alice@example.com",
    role: "admin",
    securityCode: "3F2A 91C0 7B44 E1D8",
    isCurrentUser: true,
    isKeyChanged: false,
    ...overrides,
  };
}

export function createVaultMember(overrides: Partial<VaultMember> = {}): VaultMember {
  return {
    accountId: "ALICE1",
    name: "Alice",
    email: "alice@example.com",
    role: "owner",
    securityCode: "3F2A 91C0 7B44 E1D8",
    isCurrentUser: true,
    isKeyChanged: false,
    ...overrides,
  };
}

export const testBob = { accountId: "BOB222", name: "Bob Stone", email: "bob@example.com", securityCode: "C0FF EE12 3456 789A", isCurrentUser: false };
export const testCarol = { accountId: "CAROL3", name: "Carol", email: "carol@example.com", securityCode: "0A1B 2C3D 4E5F 6071", isCurrentUser: false };

export function createDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: "device-1",
    name: "Alice's MacBook",
    platform: "darwin/desktop",
    createdAt: "2026-09-01T10:00:00Z",
    lastSeenAt: new Date().toISOString(),
    isCurrent: true,
    ...overrides,
  };
}

export function createRecoveryStatus(overrides: Partial<RecoveryStatus> = {}): RecoveryStatus {
  return { isSetUp: true, isAdmin: true, isCreatedByCurrentUser: true, createdByName: "Alice", ...overrides };
}

export function createPendingRecovery(overrides: Partial<PendingRecovery> = {}): PendingRecovery {
  return {
    accountId: "BOB222",
    name: "Bob Stone",
    email: "bob@example.com",
    submittedAt: new Date(Date.now() - 3 * 3600_000).toISOString(),
    recoverableVaultCount: 2,
    unrecoverableVaultCount: 1,
    ...overrides,
  };
}

export const allTestVault: VaultSummary[] = [
  { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2, recoverable: true, recoveryOptOut: false },
  { id: "family", name: "Family", type: "shared", role: "reader", itemCount: 0, recoverable: true, recoveryOptOut: false },
];

export function createApiMock() {
  return {
    getStatus: vi.fn(),
    signIn: vi.fn(),
    signUp: vi.fn(),
    unlock: vi.fn(),
    lock: vi.fn(),
    signOut: vi.fn(),
    reportActivity: vi.fn().mockResolvedValue(undefined),
    listAllVault: vi.fn(),
    createVault: vi.fn(),
    listAllItem: vi.fn(),
    getItem: vi.fn(),
    saveItem: vi.fn(),
    deleteItem: vi.fn(),
    deletePasskey: vi.fn(),
    generatePassword: vi.fn(),
    generateSSHKey: vi.fn(),
    importSSHKey: vi.fn(),
    listAllMember: vi.fn().mockResolvedValue([]),
    listAllVaultMember: vi.fn().mockResolvedValue([]),
    shareVault: vi.fn(),
    changeVaultMemberRole: vi.fn(),
    removeVaultMember: vi.fn(),
    inviteMember: vi.fn(),
    listAllDevice: vi.fn().mockResolvedValue([]),
    revokeDevice: vi.fn(),
    getSSHAgentStatus: vi.fn().mockResolvedValue(createSSHAgentStatus()),
    configureSSHConfig: vi.fn(),
    removeSSHConfig: vi.fn(),
    respondSSHApproval: vi.fn().mockResolvedValue(undefined),
    listAllPendingSSHApproval: vi.fn().mockResolvedValue([]),
    respondApprovalWithPassword: vi.fn().mockResolvedValue(undefined),
    getCommandLineStatus: vi.fn().mockResolvedValue(createCommandLineStatus()),
    installCommandLineTool: vi.fn(),
    uninstallCommandLineTool: vi.fn(),
    setCommandLineIntegration: vi.fn(),
    copyField: vi.fn(),
    getOneTimeCode: vi.fn(),
    copyText: vi.fn(),
    openURL: vi.fn().mockResolvedValue(undefined),
    unlockWithBiometric: vi.fn(),
    enableBiometricUnlock: vi.fn(),
    disableBiometricUnlock: vi.fn(),
    setTouchIDPromptFrame: vi.fn().mockResolvedValue(undefined),
    clearTouchIDPromptFrame: vi.fn().mockResolvedValue(undefined),
    setSSHApprovalForAllApplication: vi.fn().mockResolvedValue(undefined),
    setApprovalTouchIDFrame: vi.fn().mockResolvedValue(undefined),
    clearApprovalTouchIDFrame: vi.fn().mockResolvedValue(undefined),
    cancelTouchIDPrompt: vi.fn().mockResolvedValue(undefined),
    hasNativeTouchIDView: false,
    saveNewAccountEmergencyKit: vi.fn(),
    saveEmergencyKit: vi.fn(),
    forgetNewAccountSecretKey: vi.fn().mockResolvedValue(undefined),
    getRecoveryStatus: vi.fn().mockResolvedValue(createRecoveryStatus({ isSetUp: false })),
    setUpRecovery: vi.fn(),
    enableRecoveryForAllVault: vi.fn(),
    enableVaultRecovery: vi.fn(),
    disableVaultRecovery: vi.fn(),
    startAccountRecovery: vi.fn(),
    listAllPendingRecovery: vi.fn().mockResolvedValue([]),
    completeAccountRecovery: vi.fn(),
    recoverAccount: vi.fn(),
    getBrowserStatus: vi.fn().mockResolvedValue(createBrowserStatus()),
    removePairedBrowser: vi.fn(),
    setExactHostMatch: vi.fn(),
    cancelBrowserPairing: vi.fn().mockResolvedValue(undefined),
    getUpdateStatus: vi.fn().mockResolvedValue(createUpdateStatus()),
    checkForUpdates: vi.fn(),
    setAutomaticUpdateCheck: vi.fn(),
    listAllImportFormat: vi.fn().mockResolvedValue([]),
    chooseImportFile: vi.fn(),
    previewImport: vi.fn(),
    startImport: vi.fn(),
    continueImport: vi.fn(),
    cancelImport: vi.fn().mockResolvedValue(undefined),
    deleteImportFile: vi.fn(),
  };
}

export function createUpdateStatus(overrides: Partial<UpdateStatus> = {}): UpdateStatus {
  return { version: "0.2.1", buildNumber: "2001", isAvailable: true, isSignedIn: true, isAutomaticCheckEnabled: true, ...overrides };
}

export function createBrowserStatus(overrides: Partial<BrowserStatus> = {}): BrowserStatus {
  return { allInstallation: [], allPairedBrowser: [], isExactHostMatch: false, pendingPairing: null, ...overrides };
}

export function createImportPreview(overrides: Partial<ImportPreview> = {}): ImportPreview {
  return {
    fileName: "bitwarden_export.json",
    filePath: "/Users/alice/Downloads/bitwarden_export.json",
    format: "bitwarden-json",
    formatName: "Bitwarden JSON (not encrypted)",
    vaultId: "personal",
    vaultName: "Personal",
    itemCount: 12,
    loginCount: 9,
    secureNoteCount: 2,
    sshKeyCount: 1,
    allSkipped: [{ title: "Visa", reason: "Bitwarden cards are not supported in Hisar yet" }],
    allDuplicate: [{ title: "GitHub", username: "alice", url: "https://github.com" }],
    ...overrides,
  };
}

export function createImportProgress(overrides: Partial<ImportProgress> = {}): ImportProgress {
  return {
    fileName: "bitwarden_export.json",
    filePath: "/Users/alice/Downloads/bitwarden_export.json",
    vaultId: "personal",
    vaultName: "Personal",
    savedCount: 0,
    totalCount: 11,
    isDone: false,
    allFailed: [],
    ...overrides,
  };
}
