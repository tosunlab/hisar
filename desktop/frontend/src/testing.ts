import { vi } from "vitest";
import type { ApprovalEvent, ShareLink, ShareLinkField, BrowserStatus, CommandLineStatus, Device, ImportPreview, ImportProgress, ItemDetail, LoginItemStatus, Person, ReceivedShareInvitation, SentShareInvitation, SSHAgentStatus, Status, UpdateStatus, VaultMember, VaultSummary } from "./api";

export function createStatus(state: string): Status {
  return { state, server: "https://hisar.test", email: "alice@example.com", name: "Alice", biometricAvailable: false, biometricEnabled: false, biometricReady: false, isOffline: false, autoLockSeconds: 600 };
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
    isCompactApproval: false,
    isPhoneApproval: true,
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
    requesterKind: "app",
    appName: "Terminal",
    appIconURL: "",
    appPath: "/System/Applications/Utilities/Terminal.app",
    appSignature: "apple",
    appSigner: "Signed by Apple",
    appWarning: "",
    allChainProcess: [
      { name: "ssh", commandLine: "ssh git@git.tryhisar.com 'git-receive-pack '\\''hisar/core.git'\\'''" },
      { name: "git", commandLine: "git push origin main" },
      { name: "zsh", commandLine: "-zsh" },
    ],
    clientName: "ssh",
    clientPath: "/usr/bin/ssh",
    keyTitle: "Laptop key",
    fingerprint: "SHA256:hisarTestFingerprint",
    destinationHost: "git.tryhisar.com",
    destinationFingerprint: "SHA256:hostFingerprint",
    isForwarded: false,
    forwardedFromHost: "",
    purpose: "push",
    action: "push to",
    target: "git.tryhisar.com",
    commandLine: "git push origin main",
    repositoryPath: "~/src/core",
    hostAndPort: "git.tryhisar.com:22",
    branch: "",
    commitSubject: "",
    isRememberable: true,
    isCompact: false,
    isTouchIDApproval: true,
    isPhoneApproval: false,
    timeoutAt: "2026-10-09T12:01:00Z",
    allSecretName: [],
    moreSecretCount: 0,
    agentName: "",
    workingDirectory: "",
    rememberLabel: "",
    isRememberedByDefault: true,
    isConnectionApproval: false,
    warning: "",
    ...overrides,
  };
}

export function createAgentApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return createApprovalEvent({
    requestID: "agent-1",
    kind: "agent",
    agentName: "Claude Code",
    clientName: "hisar mcp",
    clientPath: "/opt/homebrew/bin/hisar",
    keyTitle: "",
    fingerprint: "",
    destinationHost: "",
    destinationFingerprint: "",
    purpose: "connect",
    action: "use Hisar",
    target: "",
    commandLine: "",
    repositoryPath: "",
    hostAndPort: "",
    isRememberable: false,
    isRememberedByDefault: false,
    isConnectionApproval: true,
    terminalName: "ttys003",
    ...overrides,
  });
}

export function createAgentRunApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return createAgentApprovalEvent({
    requestID: "agent-run-1",
    purpose: "run",
    action: "run a command with 2 secrets",
    commandLine: "npm run deploy -- --env staging",
    workingDirectory: "~/src/billing",
    allSecretName: ["Work › Stripe › api key", "Work › Postgres staging › password"],
    isRememberable: true,
    rememberLabel: "Allow these secrets for this agent for 1 hour",
    isConnectionApproval: false,
    ...overrides,
  });
}

export function createAgentRevealApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return createAgentApprovalEvent({
    requestID: "agent-reveal-1",
    purpose: "reveal",
    action: "read “Stripe › api key”",
    allSecretName: ["Work › Stripe › api key"],
    isRememberable: true,
    rememberLabel: "Allow this item for 1 hour",
    isConnectionApproval: false,
    warning: "The value will be visible to Claude Code and can end up in its transcript and logs.",
    ...overrides,
  });
}

export function createSigningApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return createApprovalEvent({
    requestID: "signing-1",
    clientName: "ssh-keygen",
    clientPath: "/usr/bin/ssh-keygen",
    keyTitle: "Signing key",
    destinationHost: "",
    destinationFingerprint: "",
    purpose: "sign-commit",
    action: "sign a commit",
    target: "",
    commandLine: "git commit -S -m 'Fix the login redirect'",
    hostAndPort: "",
    branch: "feature/login",
    commitSubject: "Fix the login redirect",
    isRememberable: false,
    ...overrides,
  });
}

export function createForwardedApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return createApprovalEvent({
    requestID: "forwarded-1",
    isForwarded: true,
    forwardedFromHost: "build.example.com",
    purpose: "sign-in",
    action: "sign in to",
    target: "github.com",
    commandLine: "ssh -A build.example.com",
    repositoryPath: "",
    hostAndPort: "github.com:22",
    isRememberable: false,
    ...overrides,
  });
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
    purpose: "",
    action: "",
    target: "",
    commandLine: "",
    repositoryPath: "",
    hostAndPort: "",
    isRememberable: false,
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

export function createPerson(overrides: Partial<Person> = {}): Person {
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

export function createReceivedShareInvitation(overrides: Partial<ReceivedShareInvitation> = {}): ReceivedShareInvitation {
  return {
    id: "invitation-from-carol",
    vaultId: "carol-vault",
    role: "editor",
    invitedByName: "Carol",
    invitedByEmail: "carol@example.com",
    expiresAt: "2026-10-18T09:00:00Z",
    ...overrides,
  };
}

export function createSentShareInvitation(overrides: Partial<SentShareInvitation> = {}): SentShareInvitation {
  return {
    id: "invitation-to-dave",
    vaultId: "family",
    vaultName: "Family",
    email: "dave@example.com",
    role: "reader",
    state: "pending",
    expiresAt: "2026-10-18T09:00:00Z",
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

export const allTestVault: VaultSummary[] = [
  { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2 },
  { id: "family", name: "Family", type: "shared", role: "reader", itemCount: 0 },
];

export function createApiMock() {
  return {
    getStatus: vi.fn(),
    setAutoLock: vi.fn(),
    getTheme: vi.fn().mockResolvedValue({ appearance: "system", color: "neutral" }),
    setTheme: vi.fn((appearance: string, color: string) => Promise.resolve({ appearance, color })),
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
    listAllPerson: vi.fn().mockResolvedValue([]),
    listAllVaultMember: vi.fn().mockResolvedValue([]),
    shareVault: vi.fn(),
    changeVaultMemberRole: vi.fn(),
    removeVaultMember: vi.fn(),
    invitePerson: vi.fn(),
    listAllShareInvitation: vi.fn().mockResolvedValue({ received: [], sent: [], completionError: "" }),
    acceptShareInvitation: vi.fn(),
    declineShareInvitation: vi.fn(),
    cancelShareInvitation: vi.fn(),
    listAllDevice: vi.fn().mockResolvedValue([]),
    revokeDevice: vi.fn(),
    listAllShareLinkField: vi.fn().mockResolvedValue([]),
    createShareLink: vi.fn(),
    listAllShareLink: vi.fn().mockResolvedValue([]),
    revokeShareLink: vi.fn().mockResolvedValue(undefined),
    copyShareLink: vi.fn().mockResolvedValue(undefined),
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
    copySecretReference: vi.fn(),
    openURL: vi.fn().mockResolvedValue(undefined),
    unlockWithBiometric: vi.fn(),
    enableBiometricUnlock: vi.fn(),
    disableBiometricUnlock: vi.fn(),
    setTouchIDPromptFrame: vi.fn().mockResolvedValue(undefined),
    clearTouchIDPromptFrame: vi.fn().mockResolvedValue(undefined),
    setSSHApprovalRemembered: vi.fn().mockResolvedValue(undefined),
    setCompactSSHApproval: vi.fn().mockResolvedValue(createSSHAgentStatus({ isCompactApproval: true })),
    setPhoneApproval: vi.fn().mockResolvedValue(createSSHAgentStatus({ isPhoneApproval: false })),
    fitSSHApprovalWindow: vi.fn().mockResolvedValue(undefined),
    cancelApprovalTouchID: vi.fn().mockResolvedValue(undefined),
    setApprovalTouchIDFrame: vi.fn().mockResolvedValue(undefined),
    clearApprovalTouchIDFrame: vi.fn().mockResolvedValue(undefined),
    cancelTouchIDPrompt: vi.fn().mockResolvedValue(undefined),
    hasNativeTouchIDView: false,
    saveNewAccountEmergencyKit: vi.fn(),
    saveEmergencyKit: vi.fn(),
    forgetNewAccountSecretKey: vi.fn().mockResolvedValue(undefined),
    getBrowserStatus: vi.fn().mockResolvedValue(createBrowserStatus()),
    removePairedBrowser: vi.fn(),
    setExactHostMatch: vi.fn(),
    cancelBrowserPairing: vi.fn().mockResolvedValue(undefined),
    getUpdateStatus: vi.fn().mockResolvedValue(createUpdateStatus()),
    checkForUpdates: vi.fn(),
    setAutomaticUpdateCheck: vi.fn(),
    listAllAgentVaultID: vi.fn().mockResolvedValue([]),
    setVaultAvailableToAgents: vi.fn(),
    getLoginItemStatus: vi.fn().mockResolvedValue(createLoginItemStatus()),
    setOpenAtLogin: vi.fn(),
    openLoginItemSettings: vi.fn().mockResolvedValue(undefined),
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

export function createLoginItemStatus(overrides: Partial<LoginItemStatus> = {}): LoginItemStatus {
  return { isAvailable: true, isOpenAtLogin: true, systemState: "enabled", canOpenSystemSettings: true, errorMessage: "", ...overrides };
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

export function createShareLink(overrides: Partial<ShareLink> = {}): ShareLink {
  return {
    id: "link-1",
    url: "",
    title: "GitHub",
    vaultID: "personal",
    itemID: "item-1",
    fieldLabels: ["username", "password"],
    isText: false,
    createdAt: "2026-10-10T09:00:00Z",
    expiresAt: new Date(Date.now() + 7 * 24 * 3600 * 1000).toISOString(),
    maxViews: 1,
    viewCount: 0,
    hasPassphrase: false,
    ...overrides,
  };
}

export function createShareLinkField(overrides: Partial<ShareLinkField> = {}): ShareLinkField {
  return { key: "username", label: "username", isDefault: true, isConcealed: false, ...overrides };
}
