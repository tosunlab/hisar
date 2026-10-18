import { vi } from "vitest";
import type {
  ApprovalEvent,
  FavoriteItem,
  ShareLink,
  ShareLinkField,
  BrowserStatus,
  CommandLineStatus,
  Device,
  ImportPreview,
  ImportProgress,
  ItemDetail,
  LauncherApp,
  LoginItemStatus,
  Organization,
  OrganizationInvitation,
  OrganizationMember,
  Person,
  ReceivedShareInvitation,
  SentShareInvitation,
  SSHAgentStatus,
  Status,
  Team,
  UpdateStatus,
  VaultActions,
  VaultMember,
  VaultSummary,
} from "./api";

export function createStatus(state: string): Status {
  return { state, server: "https://hisar.test", email: "alice@example.com", name: "Alice", biometricAvailable: false, biometricEnabled: false, biometricReady: false, isOffline: false, autoLockSeconds: 600, signedOutReason: "", updateRequired: null };
}

export function createItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return {
    id: "item-1",
    revision: "rev-1",
    category: "",
    title: "GitHub",
    username: "alice",
    urls: ["https://github.com"],
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
    canConvertToSecureNote: false,
    isFavorite: false,
    ...overrides,
  };
}

export function createFavoriteItem(overrides: Partial<FavoriteItem> = {}): FavoriteItem {
  return { vaultId: "personal", vaultName: "Personal", vaultType: "personal", item: createItem({ isFavorite: true }), ...overrides };
}

export function createCardItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "card-1",
    category: "credit-card",
    title: "Visa",
    username: "",
    urls: [],
    fields: [{ name: "Bank", value: "Garanti BBVA", concealed: false }],
    card: {
      cardholder: "Alice Smith",
      type: "Visa",
      number: "4111 1111 1111 1111",
      expiry: "12/29",
      securityCode: "123",
      pin: "0000",
      street: "1 Main St",
      city: "Springfield",
      region: "IL",
      postalCode: "12345",
      country: "US",
      billingAddress: "",
      brand: "Visa",
      last4: "1111",
      numberWarning: "",
      expiryWarning: "",
    },
    ...overrides,
  });
}

export function createIdentityItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "identity-1",
    category: "identity",
    title: "Alice",
    username: "",
    urls: [],
    fields: [{ name: "Employer", value: "Hisar", concealed: false }],
    identity: {
      firstName: "Alice",
      middleName: "",
      lastName: "Example",
      birthDate: "1990-04-12",
      email: "alice@example.com",
      phone: "+1 555 0100",
      street: "1 Main St\nApt 4",
      city: "Springfield",
      region: "IL",
      postalCode: "12345",
      country: "US",
      address: "",
      passportNumber: "U1234567",
      driversLicenseNumber: "",
      nationalIdNumber: "",
    },
    ...overrides,
  });
}

export function createAPICredentialItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "stripe-1",
    category: "api-credential",
    title: "Stripe",
    username: "",
    urls: [],
    fields: [{ name: "Scope", value: "read", concealed: false }],
    apiCredential: { service: "Stripe", keyId: "key_1", secret: "sk_live_123", host: "api.stripe.com", expiry: "2031-01-01", expiryWarning: "" },
    ...overrides,
  });
}

export function createAddressItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "home-1",
    category: "address",
    title: "Home",
    username: "",
    urls: [],
    fields: [{ name: "Gate code", value: "1234", concealed: true }],
    address: { street: "1 Main St\nApt 4", city: "Springfield", region: "IL", postalCode: "12345", country: "US" },
    ...overrides,
  });
}

export function createSoftwareLicenseItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "sketch-1",
    category: "software-license",
    title: "Sketch",
    username: "",
    urls: [],
    fields: [{ name: "Seats", value: "5", concealed: false }],
    softwareLicense: {
      product: "Sketch",
      version: "100",
      licenseKey: "TEST-LICENSE-KEY",
      licensedTo: "Alice Example",
      email: "alice@example.com",
      purchaseDate: "2024-03-09",
      downloadUrl: "https://www.sketch.com/download",
    },
    ...overrides,
  });
}

export function createServerItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "build-1",
    category: "server",
    title: "Build server",
    username: "deploy",
    password: "server-password",
    urls: ["https://console.example.com"],
    fields: [{ name: "Provider", value: "Example Cloud", concealed: false }],
    server: {
      host: "build.example.com",
      port: "2222",
      sshKeyId: "key-1",
      sshKeyTitle: "Deploy key",
      sshKeyFingerprint: "SHA256:abc",
      portWarning: "",
    },
    ...overrides,
  });
}

export function createDatabaseItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "db-1",
    category: "database",
    title: "Prod db",
    username: "app",
    password: "db-password",
    urls: [],
    fields: [{ name: "SID", value: "ORCL", concealed: false }],
    database: {
      engine: "postgresql",
      engineName: "PostgreSQL",
      host: "db.example.com",
      port: "5432",
      name: "app",
      options: "sslmode=require",
      connectionString: "postgresql://app:db-password@db.example.com:5432/app?sslmode=require",
      portWarning: "",
    },
    ...overrides,
  });
}

export function createSecureNoteItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return createItem({
    id: "note-1",
    category: "secure-note",
    title: "Recovery codes",
    username: "",
    urls: [],
    notes: "1. 4821-9930\n2. 1177-0042",
    ...overrides,
  });
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
    signingRememberSeconds: 0,
    sshRememberSeconds: 3600,
    backupPath: "",
    ...overrides,
  };
}

export function createApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return {
    requestID: "request-1",
    kind: "ssh-key",
    terminalName: "",
    coverage: "",
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
    rememberPeriod: "for 1 hour",
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
    coverage: "terminal-session",
    ...overrides,
  });
}

export function createLauncherApprovalEvent(overrides: Partial<ApprovalEvent> = {}): ApprovalEvent {
  return createApprovalEvent({
    requestID: "launcher-1",
    kind: "launcher",
    appName: "Raycast",
    appIconURL: "data:image/png;base64,Raycast",
    appPath: "/Applications/Raycast.app",
    appSignature: "developer",
    appSigner: "Signed by Raycast Technologies Inc (A1B2C3D4E5)",
    allChainProcess: [{ name: "node", commandLine: "node --max-old-space-size=4096" }],
    clientName: "",
    clientPath: "/Users/alice/Library/Application Support/com.raycast.macos/node/bin/node",
    keyTitle: "",
    fingerprint: "",
    destinationHost: "",
    destinationFingerprint: "",
    purpose: "",
    action: "search Hisar and copy or paste passwords and one-time codes",
    target: "",
    commandLine: "",
    repositoryPath: "",
    hostAndPort: "",
    isRememberable: false,
    coverage: "app",
    ...overrides,
  });
}

export function createLauncherApp(overrides: Partial<LauncherApp> = {}): LauncherApp {
  return { name: "Raycast", path: "/Applications/Raycast.app", iconURL: "data:image/png;base64,Raycast", lastUsedAt: "2026-10-09T11:59:30Z", ...overrides };
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
    isInstalledByPackage: false,
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
    recipientName: "",
    role: "reader",
    state: "pending",
    expiresAt: "2026-10-18T09:00:00Z",
    acceptedAt: "",
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

export function createOrganizationMember(overrides: Partial<OrganizationMember> = {}): OrganizationMember {
  return { accountId: "ALICE1", name: "Alice", email: "alice@example.com", role: "admin", securityCode: "3F2A 91C0 7B44 E1D8", isCurrentUser: true, isKeyChanged: false, ...overrides };
}

export function createOrganizationTeam(overrides: Partial<Team> = {}): Team {
  return {
    id: "platform",
    name: "Platform",
    members: [
      { accountId: "ALICE1", name: "Alice", email: "alice@example.com", isCurrentUser: true },
      { accountId: testBob.accountId, name: testBob.name, email: testBob.email, isCurrentUser: false },
    ],
    vaults: [{ vaultId: "deploy", name: "Deploy", role: "owner", canOpen: true }],
    ...overrides,
  };
}

// createOrganizationDetail is Acme as its admin Alice sees it: Bob is a member, both are in Platform, which owns Deploy.
export function createOrganizationDetail(overrides: Partial<Organization> = {}): Organization {
  return {
    id: "acme",
    name: "Acme",
    role: "admin",
    isAdmin: true,
    members: [createOrganizationMember(), createOrganizationMember({ ...testBob, role: "member" })],
    teams: [createOrganizationTeam()],
    invitations: [],
    ...overrides,
  };
}

export function createOrganizationInvitation(overrides: Partial<OrganizationInvitation> = {}): OrganizationInvitation {
  return {
    id: "invitation-to-globex",
    organizationId: "globex",
    organizationName: "Globex",
    role: "member",
    invitedByName: "Ada",
    invitedByEmail: "ada@example.com",
    expiresAt: "2026-10-18T09:00:00Z",
    ...overrides,
  };
}

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

export function createVaultActions(overrides: Partial<VaultActions> = {}): VaultActions {
  return {
    vault: { id: "work", name: "Work", type: "shared", role: "owner", itemCount: 12 },
    canRename: true,
    canDelete: true,
    deleteBlockedReason: "",
    canLeave: false,
    leaveBlockedReason: "You are its only owner. Delete the vault, or make someone else an owner first.",
    membersLosingAccess: [],
    targetVaults: [{ id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 2 }],
    ...overrides,
  };
}

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
    getVaultActions: vi.fn().mockResolvedValue(createVaultActions()),
    renameVault: vi.fn(),
    deleteVault: vi.fn(),
    leaveVault: vi.fn().mockResolvedValue(undefined),
    listAllRemovedVault: vi.fn().mockResolvedValue([]),
    getExportOptions: vi.fn().mockResolvedValue({ isBiometricAvailable: true, minPassphraseLength: 10 }),
    exportAllData: vi.fn(),
    listAllItem: vi.fn(),
    getItem: vi.fn(),
    saveItem: vi.fn(),
    deleteItem: vi.fn(),
    moveItem: vi.fn(),
    duplicateItem: vi.fn(),
    prepareItemIcon: vi.fn(),
    getWebsiteIcon: vi.fn().mockResolvedValue(""),
    isWebsiteIconsEnabled: vi.fn().mockResolvedValue(true),
    setWebsiteIconsEnabled: vi.fn().mockResolvedValue(undefined),
    listAllFavoriteItem: vi.fn().mockResolvedValue([]),
    setItemFavorite: vi.fn().mockResolvedValue(undefined),
    convertToSecureNote: vi.fn(),
    describeCard: vi.fn().mockResolvedValue({ brand: "", last4: "", numberWarning: "", expiryWarning: "" }),
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
    listAllOrganization: vi.fn().mockResolvedValue({ organizations: [], invitations: [] }),
    getOrganization: vi.fn(),
    createOrganization: vi.fn(),
    renameOrganization: vi.fn().mockResolvedValue(undefined),
    deleteOrganization: vi.fn().mockResolvedValue(undefined),
    leaveOrganization: vi.fn().mockResolvedValue(undefined),
    inviteToOrganization: vi.fn().mockResolvedValue(undefined),
    cancelOrganizationInvitation: vi.fn().mockResolvedValue(undefined),
    acceptOrganizationInvitation: vi.fn().mockResolvedValue(undefined),
    declineOrganizationInvitation: vi.fn().mockResolvedValue(undefined),
    setOrganizationMemberRole: vi.fn().mockResolvedValue(undefined),
    removeOrganizationMember: vi.fn().mockResolvedValue(undefined),
    createTeam: vi.fn(),
    renameTeam: vi.fn().mockResolvedValue(undefined),
    deleteTeam: vi.fn().mockResolvedValue(undefined),
    addTeamMember: vi.fn().mockResolvedValue(undefined),
    removeTeamMember: vi.fn().mockResolvedValue(undefined),
    giveVaultToTeam: vi.fn().mockResolvedValue(undefined),
    takeVaultFromTeam: vi.fn().mockResolvedValue(undefined),
    createOrganizationVault: vi.fn(),
    listAllWaitingVaultKey: vi.fn().mockResolvedValue({ waiting: [], deliveryError: "", rotationError: "" }),
    listAllShareAwaitingUnlock: vi.fn().mockResolvedValue([]),
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
    listAllLauncherApp: vi.fn().mockResolvedValue([]),
    revokeLauncherApp: vi.fn(),
    copyField: vi.fn(),
    getOneTimeCode: vi.fn(),
    copyText: vi.fn(),
    copySecretReference: vi.fn(),
    openURL: vi.fn().mockResolvedValue(undefined),
    unlockWithBiometric: vi.fn(),
    enableBiometricUnlock: vi.fn(),
    enableNewAccountBiometricUnlock: vi.fn(),
    disableBiometricUnlock: vi.fn(),
    setTouchIDPromptFrame: vi.fn().mockResolvedValue(undefined),
    clearTouchIDPromptFrame: vi.fn().mockResolvedValue(undefined),
    setSSHApprovalRemembered: vi.fn().mockResolvedValue(undefined),
    setCompactSSHApproval: vi.fn().mockResolvedValue(createSSHAgentStatus({ isCompactApproval: true })),
    setPhoneApproval: vi.fn().mockResolvedValue(createSSHAgentStatus({ isPhoneApproval: false })),
    setSigningRemember: vi.fn().mockResolvedValue(createSSHAgentStatus({ signingRememberSeconds: 600 })),
    setSSHRemember: vi.fn().mockResolvedValue(createSSHAgentStatus({ sshRememberSeconds: -1 })),
    fitSSHApprovalWindow: vi.fn().mockResolvedValue(undefined),
    cancelApprovalTouchID: vi.fn().mockResolvedValue(undefined),
    setApprovalTouchIDFrame: vi.fn().mockResolvedValue(undefined),
    clearApprovalTouchIDFrame: vi.fn().mockResolvedValue(undefined),
    cancelTouchIDPrompt: vi.fn().mockResolvedValue(undefined),
    hasNativeTouchIDView: false,
    saveNewAccountEmergencyKit: vi.fn(),
    saveEmergencyKit: vi.fn(),
    changeMasterPassword: vi.fn(),
    replaceSecretKey: vi.fn(),
    previewAccountDeletion: vi.fn(),
    deleteAccount: vi.fn(),
    dismissSignedOutReason: vi.fn(),
    forgetNewAccountSecretKey: vi.fn().mockResolvedValue(undefined),
    getBrowserStatus: vi.fn().mockResolvedValue(createBrowserStatus()),
    removePairedBrowser: vi.fn(),
    setExactHostMatch: vi.fn(),
    cancelBrowserPairing: vi.fn().mockResolvedValue(undefined),
    checkMinimumVersion: vi.fn(),
    takePendingInvite: vi.fn().mockResolvedValue(""),
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

export type ApiMock = ReturnType<typeof createApiMock>;

export function createUpdateStatus(overrides: Partial<UpdateStatus> = {}): UpdateStatus {
  return { version: "0.2.1", buildNumber: "2001", operatingSystem: "macOS 26.0.1", isAvailable: true, isSignedIn: true, isAutomaticCheckEnabled: true, ...overrides };
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
    creditCardCount: 0,
    identityCount: 0,
    apiCredentialCount: 0,
    addressCount: 0,
    softwareLicenseCount: 0,
    serverCount: 0,
    databaseCount: 0,
    allSkipped: [{ title: "Visa", reason: "Bitwarden cards are not supported in Hisar yet" }],
    allDuplicate: [{ title: "GitHub", username: "alice", url: "https://github.com", detail: "alice · github.com" }],
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
