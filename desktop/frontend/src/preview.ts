// Development-only stand-in for the Go backend, used with `npm run preview:ui` to view the UI in a browser.
import type {
  CardDescription,
  ApprovalEvent,
  BrowserStatus,
  CommandLineStatus,
  Device,
  ExportOptions,
  ExportRequest,
  ExportResult,
  FavoriteItem,
  RemovedVaultNotice,
  VaultActions,
  VaultDeletionRequest,
  VaultDeletionResult,
  Person,
  ImportPreview,
  ImportProgress,
  ItemDetail,
  LoginItemStatus,
  PasswordConfig,
  ShareInvitations,
  ShareLink,
  ShareLinkField,
  ShareLinkRequest,
  SignUpResult,
  SSHAgentStatus,
  SSHKeyType,
  Status,
  Theme,
  UpdateStatus,
  VaultMember,
  VaultRole,
  VaultSummary,
  Organization,
  OrganizationInvitation,
  OrganizationRole,
  OrganizationSummary,
  Team,
  TeamVault,
  WaitingVaultKeys,
} from "./api";
import { getDomain } from "./util";

const previewParameter = new URLSearchParams(window.location.search);

// &organizations=none starts outside any organization: no organization vaults, invitations or waiting keys.
const hasPreviewOrganizations = previewParameter.get("organizations") !== "none";

const allPreviewVault: VaultSummary[] = [
  { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0 },
  { id: "team", name: "Team", type: "shared", role: "owner", itemCount: 0 },
  { id: "work", name: "Work", type: "shared", role: "reader", itemCount: 0 },
  ...(hasPreviewOrganizations
    ? [
        { id: "deploy", name: "Deploy keys", type: "organization", role: "owner", itemCount: 0, organizationId: "acme" },
        { id: "staging", name: "Staging", type: "organization", role: "editor", itemCount: 0, organizationId: "acme" },
        { id: "research", name: "Research notes", type: "organization", role: "reader", itemCount: 0, organizationId: "globex" },
      ]
    : []),
];

function createPreviewItem(id: string, title: string, username: string, link: string, password: string, extra: Partial<ItemDetail> = {}): ItemDetail {
  return {
    id,
    revision: "rev",
    category: "",
    title,
    username,
    urls: link ? [link] : [],
    tags: [],
    password,
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
    ...extra,
  };
}

// ?favorites=none starts without any; otherwise a few items in different vaults are starred.
const allPreviewFavoriteItemID = new Set<string>(previewParameter.get("favorites") === "none" ? [] : ["github", "aws", "staging-database", "vpn"]);

function markPreviewFavorite(vaultItem: ItemDetail): ItemDetail {
  return { ...vaultItem, isFavorite: allPreviewFavoriteItemID.has(vaultItem.id) };
}

function createRandomText(length: number): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  return Array.from(crypto.getRandomValues(new Uint32Array(length)), (random) => alphabet[random % alphabet.length]).join("");
}

function createPreviewSSHKey(id: string, title: string, keyType: string, notes = ""): ItemDetail {
  const keyBody = keyType === "ssh-rsa" ? `AAAAB3NzaC1yc2EAAAADAQABAAACAQ${createRandomText(680)}` : `AAAAC3NzaC1lZDI1NTE5AAAAI${createRandomText(43)}`;
  return createPreviewItem(id, title, "", "", "", {
    category: "ssh-key",
    publicKey: `${keyType} ${keyBody} ${title}`,
    fingerprint: `SHA256:${createRandomText(43)}`,
    keyType,
    notes,
  });
}

const allPreviewItemByVaultID: Record<string, ItemDetail[]> = {
  personal: [
    createPreviewItem("github", "GitHub", "alper", "https://github.com", "k7#Vq2!mZp9@Lx4$Rt8w", {
      notes: "Recovery codes are in the safe.",
      fields: [{ name: "Recovery code", value: "a1b2-c3d4-e5f6", concealed: true }],
      oneTimePassword: "otpauth://totp/GitHub:alper?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&issuer=GitHub",
      hasOneTimePassword: true,
      passkeys: [{ credentialId: "preview-passkey", relyingPartyId: "github.com", userName: "alper", userDisplayName: "Alper", createdAt: "2026-10-02T09:41:00Z" }],
    }),
    createPreviewItem("google", "Google", "alper@gmail.com", "https://accounts.google.com", "Spring2024!"),
    createPreviewItem("bank", "İş Bankası", "48211990", "https://www.isbank.com.tr", "93-Kale-Kapı-71", {
      fields: [{ name: "Card PIN", value: "4821", concealed: true }],
    }),
    createPreviewItem("aws", "AWS Console", "root@tryhisar.com", "https://console.aws.amazon.com", "Hj4!pQ8&zW2^nB6*Ty1%"),
    createPreviewItem("figma", "Figma", "alper@tryhisar.com", "https://figma.com", "password1"),
    createPreviewItem("linear", "Linear", "alper@tryhisar.com", "https://linear.app", "Mx9$Lp2!Qw7@"),
    createPreviewSSHKey("github-ssh", "GitHub SSH key", "ssh-ed25519", "Also used for commit signing."),
    createPreviewSSHKey("raspberry", "Raspberry Pi", "ssh-rsa"),
    createPreviewItem("recovery-codes", "GitHub recovery codes", "", "", "", {
      category: "secure-note",
      notes: "1. 4821-9930\n2. 1177-0042\n3. 5510-3384\n\nGenerated when two-factor sign-in was turned on.",
    }),
    createPreviewItem("door-codes", "Door codes", "", "", "", {
      notes: "Front door 1234\nGarage 5678",
      canConvertToSecureNote: true,
    }),
    createPreviewItem("visa", "Garanti Bonus", "", "", "", {
      category: "credit-card",
      notes: "Main card for travel.",
      fields: [{ name: "Bank", value: "Garanti BBVA", concealed: false }],
      card: {
        cardholder: "Alper Tosun",
        type: "Visa",
        number: "4111 1111 1111 1111",
        expiry: "09/26",
        securityCode: "123",
        pin: "4821",
        street: "Bağdat Cd. 100",
        city: "İstanbul",
        region: "Kadıköy",
        postalCode: "34710",
        country: "TR",
        billingAddress: "",
        brand: "Visa",
        last4: "1111",
        numberWarning: "",
        expiryWarning: "This card has expired",
      },
    }),
    createPreviewItem("passport", "Passport", "", "", "", {
      category: "identity",
      fields: [{ name: "Expiry date", value: "2031-06-30", concealed: false }],
      identity: {
        firstName: "Alper",
        middleName: "",
        lastName: "Tosun",
        birthDate: "1990-04-12",
        email: "alper@example.com",
        phone: "+90 555 010 0000",
        street: "",
        city: "",
        region: "",
        postalCode: "",
        country: "",
        address: "Bağdat Cd. 100\nKadıköy, İstanbul",
        passportNumber: "U12345678",
        driversLicenseNumber: "",
        nationalIdNumber: "",
      },
    }),
  ],
  team: [
    createPreviewItem("sentry", "Sentry", "team@example.com", "https://sentry.io", "sentry-team-pass", { notes: "Shared team account" }),
    createPreviewItem("wifi", "Office Wi-Fi", "", "", "wifi-password-123", { fields: [{ name: "Network", value: "Hisar-Office", concealed: false }] }),
    createPreviewItem("staging-database", "Staging database", "deploy", "", "Tg5!kP9#vL2@", {
      category: "database",
      fields: [{ name: "Region", value: "eu-central", concealed: false }],
      database: {
        engine: "postgresql",
        engineName: "PostgreSQL",
        host: "db.staging.example.com",
        port: "5432",
        name: "app",
        options: "sslmode=require",
        connectionString: "postgresql://deploy:Tg5%21kP9%23vL2%40@db.staging.example.com:5432/app?sslmode=require",
        portWarning: "",
      },
    }),
    createPreviewSSHKey("deploy-server", "Deploy server", "ssh-ed25519"),
    createPreviewItem("build-server", "Build server", "deploy", "https://console.build.example.com", "Wq4!sN8#pD2@", {
      category: "server",
      fields: [{ name: "Provider", value: "Hetzner", concealed: false }],
      server: {
        host: "build.example.com",
        port: "2222",
        sshKeyId: "deploy-server",
        sshKeyTitle: "Deploy server",
        sshKeyFingerprint: "SHA256:preview-deploy-server",
        portWarning: "",
      },
    }),
  ],
  work: [
    createPreviewItem("vpn", "Office VPN", "alper", "https://vpn.example.com", "Vq8@Lm3!Zr6#"),
    createPreviewItem("stripe", "Stripe", "", "", "", {
      category: "api-credential",
      notes: "Live key for the billing service",
      fields: [{ name: "Scope", value: "charges:write", concealed: false }],
      apiCredential: {
        service: "Stripe",
        keyId: "key_preview_51H8",
        secret: "preview-secret-not-a-real-key",
        host: "api.stripe.com",
        expiry: "2027-03-31",
        expiryWarning: "",
      },
    }),
    createPreviewItem("office", "Office", "", "", "", {
      category: "address",
      fields: [{ name: "Reception", value: "Floor 3", concealed: false }],
      address: { street: "Büyükdere Cd. 185\nKat 3", city: "İstanbul", region: "Şişli", postalCode: "34394", country: "TR" },
    }),
    createPreviewItem("sketch", "Sketch", "", "", "", {
      category: "software-license",
      fields: [{ name: "Seats", value: "5", concealed: false }],
      softwareLicense: {
        product: "Sketch",
        version: "100",
        licenseKey: "PREVIEW-LICENSE-KEY",
        licensedTo: "Alper Yılmaz",
        email: "alper@tryhisar.com",
        purchaseDate: "2024-03-09",
        downloadUrl: "https://www.sketch.com/download",
      },
    }),
  ],
  deploy: [
    createPreviewSSHKey("deploy-bot", "deploy-bot", "ssh-ed25519", "Pushes release tags from CI."),
    createPreviewItem("registry", "Container registry", "ci-bot", "https://ghcr.io", "registry-pass-7Kq2Lm9X"),
  ],
  staging: [createPreviewItem("staging-postgres", "Staging Postgres", "app", "https://staging.tryhisar.com", "St4g!ng-Pg-2026")],
  research: [createPreviewItem("arxiv", "arXiv", "alper@tryhisar.com", "https://arxiv.org", "Q9!pL2x#vR4m")],
};

function getInitialState(): string {
  return new URLSearchParams(window.location.search).get("state") ?? "unlocked";
}

let previewState = getInitialState();
let previewSignedOutEmail = "alper@tryhisar.com";
let isPreviewBiometricEnabled = new URLSearchParams(window.location.search).get("touchId") === "1";
let previewAutoLockSeconds = 600;
let previewSignedOutReason =
  previewParameter.get("signedOutReason") === "1"
    ? "This device was signed out. If your master password or Secret Key was changed on another device, sign in with the new one."
    : "";
// &appearance=light|dark|system and &color=neutral|blue|purple|green pick the theme the preview starts with.
let previewTheme: Theme = { appearance: previewParameter.get("appearance") ?? "system", color: previewParameter.get("color") ?? "neutral" };
// &updateRequired=0.2.0 shows the blocking update screen for that minimum version.
const previewMinimumVersion = previewParameter.get("updateRequired");
const previewUpdateRequired = previewMinimumVersion ? { currentVersion: "0.1.0", minimumVersion: previewMinimumVersion, updateURL: "https://tryhisar.com/download" } : null;
const previewStatus = (): Status => ({
  state: previewState,
  server: "https://api.tryhisar.com",
  email: previewState === "signed-out" ? previewSignedOutEmail : "alper@tryhisar.com",
  name: "Alper",
  biometricAvailable: true,
  biometricEnabled: isPreviewBiometricEnabled,
  biometricReady: isPreviewBiometricEnabled && previewState === "locked",
  isOffline: new URLSearchParams(window.location.search).get("offline") === "1",
  autoLockSeconds: previewAutoLockSeconds,
  signedOutReason: previewState === "signed-out" ? previewSignedOutReason : "",
  updateRequired: previewUpdateRequired,
});
let isPreviewSSHConfigured = new URLSearchParams(window.location.search).get("sshConfigured") === "1";
let isPreviewCompactApproval = new URLSearchParams(window.location.search).get("compact") === "1";
let isPreviewPhoneApproval = new URLSearchParams(window.location.search).get("phoneApprovalOff") !== "1";
let previewSigningRememberSeconds = Number(new URLSearchParams(window.location.search).get("signingRemember") ?? 0);
let previewSSHRememberSeconds = Number(new URLSearchParams(window.location.search).get("sshRemember") ?? 3600);
const describePreviewSSHRememberPeriod = (): string => (previewSSHRememberSeconds < 0 ? "until Hisar locks" : "for 1 hour");
const previewAgentStatus = (backupPath = ""): SSHAgentStatus => {
  const hasError = new URLSearchParams(window.location.search).get("agent") === "error";
  return {
    socketPath: "/Users/alper/.hisar/agent.sock",
    isRunning: !hasError,
    errorMessage: hasError ? "sshagent: another agent is listening on /Users/alper/.hisar/agent.sock" : "",
    isSSHConfigured: isPreviewSSHConfigured,
    configPath: "/Users/alper/.ssh/config",
    configSection: '# Added by Hisar: use the Hisar SSH agent\nHost *\n  IdentityAgent "~/.hisar/agent.sock"\n# End of Hisar section\n',
    isTouchIDApproval: new URLSearchParams(window.location.search).get("noTouchID") !== "1",
    isCompactApproval: isPreviewCompactApproval,
    isPhoneApproval: isPreviewPhoneApproval,
    signingRememberSeconds: previewSigningRememberSeconds,
    sshRememberSeconds: previewSSHRememberSeconds,
    backupPath,
  };
};

const allPreviewPerson: Person[] = [
  { accountId: "alper", name: "Alper", email: "alper@tryhisar.com", role: "member", securityCode: "3F2A 91C0 7B44 E1D8", isCurrentUser: true, isKeyChanged: false },
  { accountId: "elif", name: "Elif Yılmaz", email: "elif@tryhisar.com", role: "admin", securityCode: "A7C2 5E19 04BD 6F3A", isCurrentUser: false, isKeyChanged: false },
  { accountId: "deniz", name: "Deniz Aydın", email: "deniz@tryhisar.com", role: "member", securityCode: "1D88 C4F0 9A27 B35E", isCurrentUser: false, isKeyChanged: false },
  { accountId: "murat", name: "Murat Kaya", email: "murat@tryhisar.com", role: "member", securityCode: "6B0E 72AD F915 3C84", isCurrentUser: false, isKeyChanged: false },
  { accountId: "zeynep", name: "Zeynep Demir", email: "zeynep@tryhisar.com", role: "member", securityCode: "E24F 8B61 0DC7 59A3", isCurrentUser: false, isKeyChanged: false },
];

const allPreviewAccessByVaultID: Record<string, { accountId: string; role: string }[]> = {
  team: [
    { accountId: "alper", role: "owner" },
    { accountId: "elif", role: "editor" },
    { accountId: "deniz", role: "reader" },
  ],
  work: [
    { accountId: "murat", role: "owner" },
    { accountId: "alper", role: "reader" },
  ],
};

const hourInMilliseconds = 3600_000;

// shareInvitations=none hides the preview invitations.
const previewShareInvitations: ShareInvitations =
  previewParameter.get("shareInvitations") === "none"
    ? { received: [], sent: [], waiting: [], completionError: "" }
    : {
        received: [
          {
            id: "from-can",
            vaultId: "infra",
            role: "editor",
            invitedByName: "Can Öztürk",
            invitedByEmail: "can@ozturk.dev",
            expiresAt: new Date(Date.now() + 6 * 24 * hourInMilliseconds).toISOString(),
          },
        ],
        sent: [
          {
            id: "to-zeynep",
            vaultId: "team",
            vaultName: "Team",
            email: "zeynep@tryhisar.com",
            recipientName: "",
            role: "reader",
            state: "pending",
            expiresAt: new Date(Date.now() + 5 * 24 * hourInMilliseconds).toISOString(),
            acceptedAt: "",
          },
          {
            id: "to-ops",
            vaultId: "team",
            vaultName: "Team",
            email: "ops@kaya.dev",
            recipientName: "Ada Kaya",
            role: "editor",
            state: "accepted",
            expiresAt: new Date(Date.now() + 2 * 24 * hourInMilliseconds).toISOString(),
            acceptedAt: new Date(Date.now() - 2 * hourInMilliseconds).toISOString(),
          },
        ],
        waiting: [
          {
            id: "from-murat",
            vaultId: "deploy",
            role: "reader",
            invitedByName: "Murat Demir",
            invitedByEmail: "murat@demir.dev",
            acceptedAt: new Date(Date.now() - 2 * hourInMilliseconds - 5 * 60_000).toISOString(),
          },
        ],
        completionError: "",
      };

function findPreviewPerson(accountID: string): Person {
  return allPreviewPerson.find((person) => person.accountId === accountID)!;
}

function createPreviewTeam(id: string, name: string, allMemberID: string[], allVault: TeamVault[]): Team {
  return {
    id,
    name,
    members: allMemberID.map((accountID) => {
      const { accountId, name: personName, email, isCurrentUser } = findPreviewPerson(accountID);
      return { accountId, name: personName, email, isCurrentUser };
    }),
    vaults: allVault,
  };
}

// Acme: Alper and Elif are admins; Platform has Deploy keys and Staging, Ops has Deploy keys and a vault Alper has no key for.
// Globex: Alper is a member of Research, which reads Research notes. &orgMember=1 makes Alper a member of Acme too.
const allPreviewOrganization: Organization[] = hasPreviewOrganizations
  ? [
      {
        id: "acme",
        name: "Acme",
        role: previewParameter.get("orgMember") === "1" ? "member" : "admin",
        isAdmin: previewParameter.get("orgMember") !== "1",
        members: [
          { ...findPreviewPerson("alper"), role: previewParameter.get("orgMember") === "1" ? "member" : "admin" },
          { ...findPreviewPerson("elif"), role: "admin" },
          { ...findPreviewPerson("deniz"), role: "member" },
          { ...findPreviewPerson("murat"), role: "member", isKeyChanged: true },
          { ...findPreviewPerson("zeynep"), role: "member" },
        ],
        teams: [
          createPreviewTeam("ops", "Ops", ["elif", "murat"], [
            { vaultId: "deploy", name: "Deploy keys", role: "reader", canOpen: true },
            { vaultId: "production", name: "", role: "owner", canOpen: false },
          ]),
          createPreviewTeam("platform", "Platform", ["alper", "deniz", "elif"], [
            { vaultId: "deploy", name: "Deploy keys", role: "owner", canOpen: true },
            { vaultId: "staging", name: "Staging", role: "editor", canOpen: true },
          ]),
        ],
        invitations: [
          {
            id: "to-can",
            email: "can@ozturk.dev",
            role: "member",
            createdAt: new Date(Date.now() - 26 * hourInMilliseconds).toISOString(),
            expiresAt: new Date(Date.now() + 6 * 24 * hourInMilliseconds).toISOString(),
          },
        ],
      },
      {
        id: "globex",
        name: "Globex",
        role: "member",
        isAdmin: false,
        members: [
          { ...findPreviewPerson("alper"), role: "member" },
          { ...findPreviewPerson("zeynep"), role: "admin" },
        ],
        teams: [createPreviewTeam("research", "Research", ["alper", "zeynep"], [{ vaultId: "research", name: "Research notes", role: "reader", canOpen: true }])],
        invitations: [],
      },
    ]
  : [];

// &orgInvitation=none hides the invitation to Initech.
let allPreviewOrganizationInvitation: OrganizationInvitation[] =
  hasPreviewOrganizations && previewParameter.get("orgInvitation") !== "none"
    ? [
        {
          id: "to-initech",
          organizationId: "initech",
          organizationName: "Initech",
          role: "member",
          invitedByName: "Ada Kaya",
          invitedByEmail: "ada@initech.dev",
          expiresAt: new Date(Date.now() + 6 * 24 * hourInMilliseconds).toISOString(),
        },
      ]
    : [];

// &waitingKeys=none hides the Globex vault waiting for its key; &keyErrors=1 shows a key Hisar could not give and a
// vault it could not re-encrypt.
const previewWaitingVaultKeys: WaitingVaultKeys = {
  waiting:
    hasPreviewOrganizations && previewParameter.get("waitingKeys") !== "none"
      ? [{ vaultId: "lab", organizationId: "globex", organizationName: "Globex", role: "editor" }]
      : [],
  deliveryError: previewParameter.get("keyErrors") === "1" ? "murat@tryhisar.com: their signing key changed since you last shared with them" : "",
  rotationError: previewParameter.get("keyErrors") === "1" ? "Production: the server refused the new key: the vault changed meanwhile" : "",
};

function findPreviewOrganization(organizationID: string): Organization {
  return allPreviewOrganization.find((organization) => organization.id === organizationID)!;
}

function findPreviewTeam(organizationID: string, teamID: string): Team {
  return findPreviewOrganization(organizationID).teams.find((team) => team.id === teamID)!;
}

function listAllPreviewOrganizationSummary(): OrganizationSummary[] {
  return allPreviewOrganization
    .map((organization) => ({ id: organization.id, name: organization.name, role: organization.role, memberCount: organization.members.length }))
    .sort((first, second) => first.name.localeCompare(second.name));
}

// buildPreviewOrganization names only the team vaults Alper has, as core does.
function buildPreviewOrganization(organizationID: string): Organization {
  const organization = structuredClone(findPreviewOrganization(organizationID));
  for (const team of organization.teams) {
    team.vaults = team.vaults.map((teamVault) => {
      const vault = allPreviewVault.find((candidate) => candidate.id === teamVault.vaultId);
      return { ...teamVault, name: vault?.name ?? "", canOpen: vault !== undefined };
    });
  }
  return organization;
}

// changePreviewOrganization refuses like the server: the owner invariant for a vault left without an owning team.
function changePreviewOrganization(change: () => void): Promise<void> {
  const before = structuredClone(allPreviewOrganization);
  change();
  const isOwnerless = allPreviewOrganization.some((organization) =>
    before
      .find((candidate) => candidate.id === organization.id)
      ?.teams.flatMap((team) => team.vaults)
      .some(
        (teamVault) =>
          teamVault.role === "owner" && !organization.teams.some((team) => team.members.length > 0 && team.vaults.some((candidate) => candidate.vaultId === teamVault.vaultId && candidate.role === "owner")),
      ),
  );
  if (isOwnerless) {
    allPreviewOrganization.splice(0, allPreviewOrganization.length, ...before);
    return Promise.reject(new Error("Nobody who can open this vault would be its owner. Give owner access to a team whose members already have the vault first."));
  }
  return resolveAfterDelay(undefined);
}

function listAllPreviewPerson(): Person[] {
  const allSharedAccountID = new Set(
    Object.values(allPreviewAccessByVaultID)
      .filter((allAccess) => allAccess.some((access) => access.accountId === "alper"))
      .flatMap((allAccess) => allAccess.map((access) => access.accountId)),
  );
  return allPreviewPerson.filter((person) => person.isCurrentUser || allSharedAccountID.has(person.accountId));
}

let allPreviewDevice: Device[] = [
  { id: "macbook", name: "Alper's MacBook Pro", platform: "darwin/desktop", createdAt: "2026-08-01T09:00:00Z", lastSeenAt: new Date().toISOString(), isCurrent: true },
  { id: "mac-mini", name: "Mac mini", platform: "darwin/desktop", createdAt: "2026-08-12T09:00:00Z", lastSeenAt: new Date(Date.now() - 2 * hourInMilliseconds).toISOString(), isCurrent: false },
  { id: "dev-box", name: "alper-dev", platform: "linux/cli", createdAt: "2026-09-03T09:00:00Z", lastSeenAt: new Date(Date.now() - 5 * 24 * hourInMilliseconds).toISOString(), isCurrent: false },
];

function listAllPreviewVaultMember(vaultID: string): VaultMember[] {
  const roleOrder = ["owner", "editor", "reader"];
  return (allPreviewAccessByVaultID[vaultID] ?? [])
    .map((access) => {
      const member = allPreviewPerson.find((candidate) => candidate.accountId === access.accountId)!;
      return { ...member, role: access.role };
    })
    .sort((first, second) => roleOrder.indexOf(first.role) - roleOrder.indexOf(second.role));
}

function findPreviewAccess(vaultID: string, accountID: string) {
  return (allPreviewAccessByVaultID[vaultID] ?? []).find((access) => access.accountId === accountID);
}

function addPreviewItem(vaultID: string, detail: ItemDetail): ItemDetail {
  allPreviewItemByVaultID[vaultID].push(detail);
  return detail;
}

let allPreviewAgentVaultID: string[] = ["work"];

function buildPreviewVaultSummary(vault: VaultSummary): VaultSummary {
  return { ...vault, itemCount: allPreviewItemByVaultID[vault.id]?.length ?? 0 };
}

function buildPreviewVaultActions(vaultID: string): VaultActions {
  const vault = allPreviewVault.find((candidate) => candidate.id === vaultID)!;
  const allMember = listAllPreviewVaultMember(vaultID);
  const isOwner = vault.role === "owner";
  const ownerCount = allMember.filter((member) => member.role === "owner").length;
  let deleteBlockedReason = "";
  if (vault.type === "personal") {
    deleteBlockedReason = "Your Personal vault can't be deleted. It is the vault Hisar keeps for you alone, and logins saved from the browser and the iPhone go there.";
  } else if (!isOwner) {
    deleteBlockedReason = "Only an owner of this vault can delete it. You can leave it instead.";
  }
  let leaveBlockedReason = "";
  if (vault.type === "personal") {
    leaveBlockedReason = "You can't leave your Personal vault.";
  } else if (vault.organizationId) {
    leaveBlockedReason = "You have this vault through your teams in its organization. To give up access, ask an organization admin to change them.";
  } else if (isOwner && ownerCount <= 1) {
    leaveBlockedReason = "You are its only owner. Delete the vault, or make someone else an owner first.";
  }
  return {
    vault: buildPreviewVaultSummary(vault),
    canRename: isOwner,
    canDelete: deleteBlockedReason === "",
    deleteBlockedReason,
    canLeave: leaveBlockedReason === "",
    leaveBlockedReason,
    membersLosingAccess: allMember.filter((member) => !member.isCurrentUser),
    targetVaults:
      deleteBlockedReason === ""
        ? allPreviewVault.filter((candidate) => candidate.id !== vaultID && candidate.role !== "reader").map(buildPreviewVaultSummary)
        : [],
  };
}

function removePreviewVault(vaultID: string): void {
  allPreviewVault.splice(
    allPreviewVault.findIndex((vault) => vault.id === vaultID),
    1,
  );
  delete allPreviewItemByVaultID[vaultID];
  delete allPreviewAccessByVaultID[vaultID];
}

// &removedVault=1 shows the notice for a vault someone else deleted, once.
let allPendingPreviewRemovedVault: RemovedVaultNotice[] =
  previewParameter.get("removedVault") === "1"
    ? [{ id: "infra", name: "Infra", message: "“Infra” is no longer available: it was deleted, or you no longer have access to it." }]
    : [];

function takeAllPreviewRemovedVault(): RemovedVaultNotice[] {
  const allNotice = allPendingPreviewRemovedVault;
  allPendingPreviewRemovedVault = [];
  return allNotice;
}

function exportPreviewData(request: ExportRequest): Promise<ExportResult> {
  if (request.password && request.password.length < 4) {
    return Promise.reject(new Error("incorrect email, master password or Secret Key"));
  }
  const itemCount = Object.values(allPreviewItemByVaultID).reduce((count, allItem) => count + allItem.length, 0);
  const fileName = request.format === "plain" ? "Hisar export 2026-10-11 (not encrypted).json" : "Hisar export 2026-10-11.hisar";
  return resolveAfterDelay({ path: `/Users/alper/Documents/${fileName}`, vaultCount: allPreviewVault.length, itemCount });
}

const resolveAfterDelay = <Value>(value: Value): Promise<Value> => new Promise((resolve) => setTimeout(() => resolve(value), 80));

function createPreviewTerminalRequester(): Pick<
  ApprovalEvent,
  "requesterKind" | "appName" | "appIconURL" | "appPath" | "appSignature" | "appSigner" | "appWarning" | "allChainProcess"
> {
  return {
    requesterKind: "app",
    appName: "Terminal",
    appIconURL: "",
    appPath: "/System/Applications/Utilities/Terminal.app",
    appSignature: "apple",
    appSigner: "Signed by Apple",
    appWarning: "",
    allChainProcess: [
      { name: "ssh", commandLine: "ssh git@git.tryhisar.com 'git-receive-pack '\\''hisar/core.git'\\''" },
      { name: "git", commandLine: "git push origin main" },
      { name: "zsh", commandLine: "-zsh" },
    ],
  };
}

const allPreviewSigningChainProcess = [
  { name: "ssh-keygen", commandLine: "/usr/bin/ssh-keygen -Y sign -n git -f /Users/developer/.ssh/signing.pub /var/folders/x/.git_signing_buffer_tmpA1b2" },
  { name: "git", commandLine: "git commit -S -m 'Name the app behind every approval'" },
];

// applyPreviewApp: &app=iterm (a commit signed in iTerm2), =vscode (Visual Studio Code's git), =unsigned (an "iTerm2" without a signature),
// =background or =ssh (no app), with &appIcon=<url> for the icon.
function applyPreviewApp(event: ApprovalEvent): ApprovalEvent {
  const appIconURL = previewParameter.get("appIcon") ?? "";
  const allPreviewSigningChain =
    event.kind === "ssh-key" ? allPreviewSigningChainProcess : [{ name: "hisar", commandLine: `hisar run -- ${event.commandLine || "hisar status"}` }];
  switch (previewParameter.get("app")) {
    case "iterm":
      return {
        ...event,
        appName: "iTerm2",
        appIconURL,
        appPath: "/Applications/iTerm.app",
        appSignature: "developer",
        appSigner: "Signed by George Nachman (H7V7XYVQ7D)",
        allChainProcess: [...allPreviewSigningChain, { name: "zsh", commandLine: "-zsh" }],
      };
    case "vscode":
      return {
        ...event,
        appName: "Visual Studio Code",
        appIconURL,
        appPath: "/Applications/Visual Studio Code.app",
        appSignature: "developer",
        appSigner: "Signed by Microsoft Corporation (UBF8T346G9)",
        allChainProcess: allPreviewSigningChain,
      };
    case "unsigned":
      return {
        ...event,
        appName: "iTerm2",
        appIconURL,
        appPath: "/Users/developer/Downloads/iTerm.app",
        appSignature: "unsigned",
        appSigner: "Not signed",
        appWarning: "iTerm2 is not signed, so macOS cannot confirm who made it. Approve only if you started this yourself.",
        allChainProcess: [...allPreviewSigningChain, { name: "zsh", commandLine: "-zsh" }],
      };
    case "background":
      return { ...event, requesterKind: "background", appName: "A background process", appIconURL: "", appPath: "", appSignature: "", appSigner: "", allChainProcess: [...allPreviewSigningChain, { name: "nightly-release", commandLine: "/Users/developer/bin/nightly-release" }] };
    case "ssh":
      return { ...event, requesterKind: "ssh-session", appName: "An SSH session from 192.0.2.10", appIconURL: "", appPath: "", appSignature: "", appSigner: "", allChainProcess: [...allPreviewSigningChain, { name: "zsh", commandLine: "-zsh" }] };
  }
  return event;
}

const dayInMilliseconds = 24 * hourInMilliseconds;

function createPreviewShareLink(overrides: Partial<ShareLink>): ShareLink {
  return {
    id: `link-${Math.random().toString(36).slice(2, 10)}`,
    url: "",
    title: "",
    vaultID: "",
    itemID: "",
    fieldLabels: [],
    isText: false,
    createdAt: new Date(Date.now() - hourInMilliseconds).toISOString(),
    expiresAt: new Date(Date.now() + 7 * dayInMilliseconds).toISOString(),
    maxViews: 1,
    viewCount: 0,
    hasPassphrase: false,
    ...overrides,
  };
}

let allPreviewShareLink: ShareLink[] = [
  createPreviewShareLink({ id: "link-github", title: "GitHub", vaultID: "personal", itemID: "github", fieldLabels: ["username", "password"] }),
  createPreviewShareLink({
    id: "link-aws",
    title: "AWS Console",
    vaultID: "personal",
    itemID: "aws",
    fieldLabels: ["username", "password", "website"],
    maxViews: 5,
    viewCount: 2,
    hasPassphrase: true,
    expiresAt: new Date(Date.now() + 20 * hourInMilliseconds).toISOString(),
  }),
  createPreviewShareLink({ id: "link-text", title: "Staging database URL", fieldLabels: ["text"], isText: true, maxViews: 0, viewCount: 3, expiresAt: new Date(Date.now() + 50 * 60_000).toISOString() }),
];

function listAllPreviewShareLinkField(detail: ItemDetail): ShareLinkField[] {
  const allField: ShareLinkField[] = [];
  if (detail.username) {
    allField.push({ key: "username", label: "username", isDefault: true, isConcealed: false });
  }
  if (detail.password) {
    allField.push({ key: "password", label: "password", isDefault: true, isConcealed: true });
  }
  if (detail.urls.length > 0) {
    allField.push({ key: "urls", label: detail.urls.length === 1 ? "website" : "websites", isDefault: false, isConcealed: false });
  }
  detail.fields.forEach((field, index) => {
    if (field.value) {
      allField.push({ key: `field:${index}`, label: field.name, isDefault: false, isConcealed: field.concealed });
    }
  });
  if (detail.notes) {
    allField.push({ key: "notes", label: "notes", isDefault: false, isConcealed: false });
  }
  return allField;
}

function createPreviewShareLinkFromRequest(request: ShareLinkRequest): ShareLink {
  const linkKey = createRandomText(43).replaceAll("+", "-").replaceAll("/", "_");
  const shareLinkID = createRandomText(22).replaceAll("+", "-").replaceAll("/", "_");
  const common = {
    id: shareLinkID,
    url: `https://api.tryhisar.com/s/${shareLinkID}#${linkKey}`,
    createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + request.expiresInSeconds * 1000).toISOString(),
    maxViews: request.maxViews,
    hasPassphrase: request.passphrase !== "",
  };
  if (!request.itemID) {
    return createPreviewShareLink({ ...common, title: request.title || "Shared text", fieldLabels: ["text"], isText: true });
  }
  const detail = allPreviewItemByVaultID[request.vaultID].find((vaultItem) => vaultItem.id === request.itemID)!;
  const allField = listAllPreviewShareLinkField(detail);
  const fieldLabels = (request.fieldKeys ?? []).map((key) => allField.find((field) => field.key === key)?.label ?? key);
  return createPreviewShareLink({ ...common, title: detail.title, vaultID: request.vaultID, itemID: request.itemID, fieldLabels });
}

// createPreviewCommandLineApproval: &approval=cli, plus &cliLocked=1 (Touch ID unlocks) or &cliPassword=1 (master password unlocks).
function createPreviewCommandLineApproval(): ApprovalEvent {
  const isLocked = previewParameter.get("cliLocked") === "1" || previewParameter.get("cliPassword") === "1";
  return {
    requestID: "preview-command-line",
    kind: "command-line",
    ...createPreviewTerminalRequester(),
    clientName: "hisar",
    clientPath: "/Applications/Hisar.app/Contents/Helpers/hisar",
    keyTitle: "",
    fingerprint: "",
    destinationHost: "",
    destinationFingerprint: "",
    isForwarded: false,
    forwardedFromHost: "",
    purpose: "",
    action: "",
    target: "",
    commandLine: "",
    repositoryPath: "",
    hostAndPort: "",
    branch: "",
    commitSubject: "",
    isRememberable: false,
    isCompact: false,
    isTouchIDApproval: previewParameter.get("noTouchID") !== "1",
    timeoutAt: new Date(Date.now() + 60_000).toISOString(),
    terminalName: "ttys003",
    isUnlockRequired: isLocked,
    isPasswordUnlock: previewParameter.get("cliPassword") === "1",
    isPhoneApproval: previewParameter.get("phone") === "1" && !isLocked,
    allSecretName: [],
    moreSecretCount: 0,
    agentName: "",
    workingDirectory: "",
    rememberLabel: "",
    isRememberedByDefault: true,
    rememberPeriod: describePreviewSSHRememberPeriod(),
    isConnectionApproval: false,
    warning: "",
  };
}

// createPreviewSSHApproval is "Terminal wants to push to git.tryhisar.com"; the parameters below change it.
function createPreviewSSHApproval(): ApprovalEvent {
  const isLocked = previewParameter.get("approvalLocked") === "1" || previewParameter.get("approvalPassword") === "1";
  const isTouchIDApproval = previewParameter.get("noTouchID") !== "1";
  const isForwarded = previewParameter.get("forwarded") === "1";
  return {
    requestID: "preview-request",
    kind: "ssh-key",
    terminalName: "",
    isUnlockRequired: isLocked,
    isPasswordUnlock: previewParameter.get("approvalPassword") === "1",
    ...createPreviewTerminalRequester(),
    clientName: "ssh",
    clientPath: "/usr/bin/ssh",
    keyTitle: isLocked && previewParameter.get("approvalKey") !== "1" ? "" : "GitHub SSH key",
    fingerprint: "SHA256:Jx3o1VZ0kq7vZr6S8m2E3yQ2Lw7bQb4pN5uD9cF1aKs",
    destinationHost: "git.tryhisar.com",
    destinationFingerprint: "SHA256:+DiY3wvvV6TuJJhbpZisF/zLDA0zPMSvHdkr4UvCOqU",
    isForwarded,
    forwardedFromHost: isForwarded ? "build.tryhisar.dev" : "",
    purpose: isForwarded ? "sign-in" : "push",
    action: isForwarded ? "sign in to" : "push to",
    target: isForwarded ? "github.com" : "git.tryhisar.com",
    commandLine: isForwarded ? "ssh -A deniz@build.tryhisar.dev" : "git push origin main",
    repositoryPath: isForwarded ? "" : "~/src/hisar/core",
    hostAndPort: isForwarded ? "github.com:22" : "git.tryhisar.com:22",
    branch: "",
    commitSubject: "",
    isRememberable: !isForwarded,
    isCompact: previewParameter.get("compact") === "1" && isTouchIDApproval && !isLocked && !isForwarded,
    isTouchIDApproval,
    isPhoneApproval: previewParameter.get("phone") === "1" && !isLocked,
    timeoutAt: new Date(Date.now() + 60_000).toISOString(),
    allSecretName: [],
    moreSecretCount: 0,
    agentName: "",
    workingDirectory: "",
    rememberLabel: "",
    isRememberedByDefault: true,
    rememberPeriod: describePreviewSSHRememberPeriod(),
    isConnectionApproval: false,
    warning: "",
  };
}

// createPreviewAgentApproval: &approval=agent (hisar mcp connects), =agent-run (run_with_secrets), =agent-reveal (get_secret),
// =agent-code (get_one_time_code), plus &agentLocked=1 (master password unlocks first).
function createPreviewAgentApproval(kind: string): ApprovalEvent {
  const isLocked = previewParameter.get("agentLocked") === "1";
  const event: ApprovalEvent = {
    ...createPreviewCommandLineApproval(),
    requestID: `preview-agent-${kind}`,
    kind: "agent",
    agentName: "Claude Code",
    clientName: "hisar mcp",
    clientPath: "/opt/homebrew/bin/hisar",
    purpose: "connect",
    action: "use Hisar",
    isConnectionApproval: true,
    isRememberedByDefault: false,
    rememberPeriod: describePreviewSSHRememberPeriod(),
    isUnlockRequired: isLocked,
    isPasswordUnlock: isLocked,
    isPhoneApproval: previewParameter.get("phone") === "1" && !isLocked,
  };
  if (kind === "run") {
    return {
      ...event,
      purpose: "run",
      action: "run a command with 2 secrets",
      commandLine: "npm run db:migrate -- --env staging",
      workingDirectory: "~/src/billing-api",
      allSecretName: ["Work › Postgres staging › password", "Work › Stripe › api key"],
      isRememberable: true,
      rememberLabel: "Allow these secrets for this agent for 1 hour",
      isConnectionApproval: false,
    };
  }
  if (kind === "reveal") {
    return {
      ...event,
      purpose: "reveal",
      action: "read “Stripe › api key”",
      allSecretName: ["Work › Stripe › api key"],
      isRememberable: true,
      rememberLabel: "Allow this item for 1 hour",
      isConnectionApproval: false,
      warning: "The value will be visible to Claude Code and can end up in its transcript and logs.",
    };
  }
  if (kind === "code") {
    return {
      ...event,
      purpose: "code",
      action: "get the one-time code of “GitHub”",
      allSecretName: ["Personal › GitHub › one-time code"],
      isRememberable: true,
      rememberLabel: "Allow this one-time code for this agent for 1 hour",
      isConnectionApproval: false,
    };
  }
  return event;
}

// listAllPreviewApproval: ?window=ssh-approval&approval=1 (one request), =2 (two waiting), =signing (a commit signature), =cli (command-line tool),
// =run (hisar run with secrets),
// plus &approvalLocked=1 (Touch ID unlocks), &approvalPassword=1 (master password unlocks), &compact=1, &forwarded=1, &noTouchID=1,
// &phone=1 (the account's iPhone can approve too).
function listAllPreviewApproval(): ApprovalEvent[] {
  return listAllPreviewApprovalWithoutApp().map(applyPreviewApp);
}

function listAllPreviewApprovalWithoutApp(): ApprovalEvent[] {
  if (previewParameter.get("approval") === "cli") {
    return [createPreviewCommandLineApproval()];
  }
  const approvalParameter = previewParameter.get("approval") ?? "";
  if (approvalParameter === "agent" || approvalParameter.startsWith("agent-")) {
    return [createPreviewAgentApproval(approvalParameter.replace(/^agent-?/, ""))];
  }
  if (previewParameter.get("approval") === "run") {
    return [
      {
        ...createPreviewCommandLineApproval(),
        action: "run a command with 6 secrets",
        commandLine: "docker compose up --build",
        allSecretName: ["Personal › GitHub › API token", "Work › AWS prod › access key ID", "Work › AWS prod › secret access key", "Work › Postgres staging › password"],
        moreSecretCount: 2,
      },
    ];
  }
  const firstApproval = createPreviewSSHApproval();
  if (previewParameter.get("approval") === "signing") {
    return [
      {
        ...firstApproval,
        clientName: "ssh-keygen",
        clientPath: "/usr/bin/ssh-keygen",
        keyTitle: "Commit signing key",
        purpose: "sign-commit",
        action: "sign a commit",
        target: "",
        commandLine: "git commit -S -m 'Ask before every SSH signature'",
        hostAndPort: "",
        branch: "desktop/ssh-approval-redesign",
        commitSubject: "Ask before every SSH signature",
        isRememberable: previewSigningRememberSeconds > 0,
        isRememberedByDefault: false,
        rememberLabel:
          previewSigningRememberSeconds > 0
            ? `Don’t ask again for ${firstApproval.appName} in ${firstApproval.repositoryPath} for ${previewSigningRememberSeconds >= 3600 ? "1 hour" : "10 minutes"}`
            : "",
      },
    ];
  }
  const approvalCount = Number(previewParameter.get("approval") ?? "0");
  const allApproval = approvalCount >= 1 ? [firstApproval] : [];
  if (approvalCount >= 2) {
    allApproval.push({ ...firstApproval, requestID: "preview-request-2", appName: "Visual Studio Code", clientName: "git", clientPath: "/usr/bin/git", keyTitle: "Deploy key" });
  }
  return allApproval;
}

const hasNoPreviewBrowser = previewParameter.get("browsers") === "none";
const previewBrowserStatus: BrowserStatus = {
  allInstallation: hasNoPreviewBrowser
    ? []
    : [
        { name: "Google Chrome", manifestPath: "", isInstalled: true, errorMessage: "" },
        { name: "Google Chrome Beta", manifestPath: "", isInstalled: true, errorMessage: "" },
        { name: "Google Chrome Canary", manifestPath: "", isInstalled: true, errorMessage: "" },
        { name: "Chromium", manifestPath: "", isInstalled: true, errorMessage: "" },
        { name: "Brave", manifestPath: "", isInstalled: true, errorMessage: "" },
        { name: "Microsoft Edge", manifestPath: "", isInstalled: true, errorMessage: "" },
        { name: "Arc", manifestPath: "", isInstalled: true, errorMessage: "" },
        { name: "Vivaldi", manifestPath: "", isInstalled: false, errorMessage: "permission denied" },
      ],
  allPairedBrowser: hasNoPreviewBrowser
    ? []
    : [{ instanceID: "chrome-preview", browser: "Chrome", pairedAt: "2026-10-01T09:00:00Z", lastUsedAt: new Date(Date.now() - 3 * 3600_000).toISOString() }],
  isExactHostMatch: false,
  pendingPairing:
    previewParameter.get("browserPairing") === "1" ? { browser: "Arc", code: "482913", expiresAt: new Date(Date.now() + 110_000).toISOString() } : null,
};

// updates=development: a build without Sparkle; updates=signed-out: no session to check with.
const previewUpdateMode = previewParameter.get("updates") ?? "";
const previewUpdateStatus: UpdateStatus = {
  version: previewUpdateMode === "development" ? "development" : "0.2.1",
  buildNumber: previewUpdateMode === "development" ? "" : "2001",
  operatingSystem: "macOS 26.0.1",
  isAvailable: previewUpdateMode !== "development",
  isSignedIn: previewUpdateMode !== "signed-out",
  isAutomaticCheckEnabled: true,
};

// loginItem=requires-approval | not-registered | off | development
const previewLoginItemMode = previewParameter.get("loginItem") ?? "";
const previewLoginItemStatus: LoginItemStatus = {
  isAvailable: previewLoginItemMode !== "development",
  isOpenAtLogin: previewLoginItemMode !== "off" && previewLoginItemMode !== "development",
  systemState:
    previewLoginItemMode === "development"
      ? "unavailable"
      : previewLoginItemMode === "requires-approval"
        ? "requires-approval"
        : previewLoginItemMode === "not-registered" || previewLoginItemMode === "off"
          ? "not-registered"
          : "enabled",
  canOpenSystemSettings: previewLoginItemMode !== "development",
  errorMessage: "",
};

// commandLine=not-installed | conflict (a foreign hisar in /opt/homebrew/bin) | path-hint (~/.local/bin, not on the PATH) | development | off
const previewCommandLineMode = previewParameter.get("commandLine") ?? "installed";
const previewBundledPath = "/Applications/Hisar.app/Contents/Helpers/hisar";

function buildPreviewCommandLineStatus(isInstalled: boolean): CommandLineStatus {
  const status: CommandLineStatus = {
    isAvailable: previewCommandLineMode !== "development",
    bundledPath: previewBundledPath,
    installedPath: "",
    allInstallation: [],
    pathCommandPath: "",
    pathCommandTarget: "",
    isPathCommandFromThisApp: false,
    pathHint: "",
    isIntegrationEnabled: previewCommandLineMode !== "off",
    socketPath: "/Users/alper/.hisar/cli.sock",
    isServing: previewCommandLineMode !== "off",
    errorMessage: "",
  };
  if (previewCommandLineMode === "conflict") {
    status.allInstallation = [{ path: "/opt/homebrew/bin/hisar", kind: "other", target: "", isSymlink: false, description: "a file that is not from Hisar" }];
    status.pathCommandPath = "/opt/homebrew/bin/hisar";
    status.pathCommandTarget = "/opt/homebrew/bin/hisar";
    return status;
  }
  if (!isInstalled || !status.isAvailable) {
    return status;
  }
  const installedPath = previewCommandLineMode === "path-hint" ? "/Users/alper/.local/bin/hisar" : "/opt/homebrew/bin/hisar";
  status.installedPath = installedPath;
  status.allInstallation = [{ path: installedPath, kind: "this-app", target: previewBundledPath, isSymlink: true, description: "this app's command-line tool" }];
  if (previewCommandLineMode === "path-hint") {
    status.pathHint = `echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc`;
    return status;
  }
  status.pathCommandPath = installedPath;
  status.pathCommandTarget = previewBundledPath;
  status.isPathCommandFromThisApp = true;
  return status;
}

let isPreviewCommandLineInstalled = previewCommandLineMode !== "not-installed";
let previewCommandLineStatus = buildPreviewCommandLineStatus(isPreviewCommandLineInstalled);

const previewImportFilePath = "/Users/alper/Downloads/1PasswordExport-AQ7X2LMK-20261009-101512.1pux";

const allPreviewImportItem: ItemDetail[] = [
  createPreviewItem("import-github", "GitHub", "alper", "https://github.com", "imported-1"),
  createPreviewItem("import-google", "Google", "alper@gmail.com", "https://accounts.google.com", "imported-2"),
  createPreviewItem("import-figma", "Figma", "alper@tryhisar.com", "https://figma.com", "imported-3"),
  createPreviewItem("import-vercel", "Vercel", "alper@tryhisar.com", "https://vercel.com", "imported-4"),
  createPreviewItem("import-dropbox", "Dropbox", "alper@tryhisar.com", "https://dropbox.com", "imported-5"),
  createPreviewItem("import-notion", "Notion", "alper@tryhisar.com", "https://notion.so", "imported-6"),
  createPreviewItem("import-slack", "Slack", "alper@tryhisar.com", "https://tryhisar.slack.com", "imported-7"),
  createPreviewItem("import-amazon", "Amazon", "alper@gmail.com", "https://amazon.com.tr", "imported-8"),
  createPreviewItem("import-apple", "Apple Account", "alper@icloud.com", "https://account.apple.com", "imported-9"),
  createPreviewItem("import-hetzner", "Hetzner Cloud", "alper@tryhisar.com", "https://console.hetzner.cloud", "imported-10"),
  createPreviewItem("import-cloudflare", "Cloudflare", "alper@tryhisar.com", "https://dash.cloudflare.com", "imported-11"),
  createPreviewItem("import-trendyol", "Trendyol", "alper@gmail.com", "https://trendyol.com", "imported-12"),
  createPreviewItem("import-turkcell", "Turkcell", "5321234567", "https://turkcell.com.tr", "imported-13"),
  createPreviewItem("import-garanti", "Garanti BBVA", "48211990", "https://garantibbva.com.tr", "imported-14"),
  createPreviewItem("import-cabin", "Wi-Fi at the cabin", "", "", "", { category: "secure-note", notes: "Network: Kale\nKey: under the mat" }),
  createPreviewItem("import-codes", "Server recovery codes", "", "", "", { category: "secure-note", notes: "1. 4821-9930\n2. 1177-0042" }),
  createPreviewItem("import-insurance", "Car insurance", "", "", "", { category: "secure-note", notes: "Policy 88-120-4471, renews in March" }),
  createPreviewSSHKey("import-deploy", "Deploy key", "ssh-ed25519", "Used by CI."),
];

const allPreviewImportSkipped = [
  { title: "Visa", reason: "1Password Credit Card items are not supported in Hisar yet" },
  { title: "Personal details", reason: "1Password Identity items are not supported in Hisar yet" },
  { title: "Passport scan", reason: "1Password Document items are not supported in Hisar yet" },
  { title: "Old forum", reason: "archived in 1Password" },
];

function buildPreviewDuplicateKey(detail: ItemDetail): string {
  const domain = detail.urls[0] ? new URL(detail.urls[0]).hostname.replace(/^www\./, "") : "";
  return [detail.title.toLowerCase(), detail.username.toLowerCase(), domain].join("\n");
}

function findAllPreviewImportDuplicate(vaultID: string): ItemDetail[] {
  const allExistingKey = new Set((allPreviewItemByVaultID[vaultID] ?? []).map(buildPreviewDuplicateKey));
  return allPreviewImportItem.filter((detail) => allExistingKey.has(buildPreviewDuplicateKey(detail)));
}

function buildPreviewImportPreview(vaultID: string): ImportPreview {
  const vault = allPreviewVault.find((candidate) => candidate.id === vaultID)!;
  return {
    fileName: previewImportFilePath.split("/").pop()!,
    filePath: previewImportFilePath,
    format: "1pux",
    formatName: "1Password export (.1pux)",
    vaultId: vaultID,
    vaultName: vault.name,
    itemCount: allPreviewImportItem.length,
    loginCount: allPreviewImportItem.filter((detail) => detail.category === "").length,
    secureNoteCount: allPreviewImportItem.filter((detail) => detail.category === "secure-note").length,
    sshKeyCount: allPreviewImportItem.filter((detail) => detail.category === "ssh-key").length,
    creditCardCount: allPreviewImportItem.filter((detail) => detail.category === "credit-card").length,
    identityCount: allPreviewImportItem.filter((detail) => detail.category === "identity").length,
    apiCredentialCount: allPreviewImportItem.filter((detail) => detail.category === "api-credential").length,
    addressCount: allPreviewImportItem.filter((detail) => detail.category === "address").length,
    softwareLicenseCount: allPreviewImportItem.filter((detail) => detail.category === "software-license").length,
    serverCount: allPreviewImportItem.filter((detail) => detail.category === "server").length,
    databaseCount: allPreviewImportItem.filter((detail) => detail.category === "database").length,
    allSkipped: allPreviewImportSkipped,
    allDuplicate: findAllPreviewImportDuplicate(vaultID).map((detail) => ({
      title: detail.title,
      username: detail.username,
      url: detail.urls[0] ?? "",
      detail: [detail.username, detail.urls[0] ? getDomain(detail.urls[0]) : ""].filter(Boolean).join(" · "),
    })),
  };
}

let previewImportRun = { vaultID: "", allQueued: [] as ItemDetail[], savedCount: 0 };

function buildPreviewImportProgress(): ImportProgress {
  const vault = allPreviewVault.find((candidate) => candidate.id === previewImportRun.vaultID)!;
  return {
    fileName: previewImportFilePath.split("/").pop()!,
    filePath: previewImportFilePath,
    vaultId: vault.id,
    vaultName: vault.name,
    savedCount: previewImportRun.savedCount,
    totalCount: previewImportRun.savedCount + previewImportRun.allQueued.length,
    isDone: previewImportRun.allQueued.length === 0,
    allFailed: [],
  };
}

// continuePreviewImport saves four items per call; ?import=saving stops halfway so the progress stays on screen.
function continuePreviewImport(): Promise<ImportProgress> {
  if (previewParameter.get("import") === "saving" && previewImportRun.savedCount >= 8) {
    return new Promise(() => undefined);
  }
  return new Promise((resolve) =>
    setTimeout(() => {
      for (const detail of previewImportRun.allQueued.splice(0, 4)) {
        allPreviewItemByVaultID[previewImportRun.vaultID].push({ ...structuredClone(detail), id: `${detail.id}-${Date.now()}` });
        previewImportRun.savedCount++;
      }
      resolve(buildPreviewImportProgress());
    }, 350),
  );
}

export const previewApi = {
  listAllImportFormat: () =>
    resolveAfterDelay([
      { id: "1pux", name: "1Password export (.1pux)" },
      { id: "1password-csv", name: "1Password CSV" },
      { id: "bitwarden-json", name: "Bitwarden JSON (not encrypted)" },
      { id: "bitwarden-csv", name: "Bitwarden CSV" },
      { id: "csv", name: "CSV (Chrome, Firefox, Safari, Edge and others)" },
    ]),
  chooseImportFile: (vaultID: string) => resolveAfterDelay(buildPreviewImportPreview(vaultID)),
  previewImport: (vaultID: string) => resolveAfterDelay(buildPreviewImportPreview(vaultID)),
  startImport: (vaultID: string, isSkippingDuplicate: boolean) => {
    const allDuplicate = isSkippingDuplicate ? findAllPreviewImportDuplicate(vaultID) : [];
    previewImportRun = { vaultID, allQueued: allPreviewImportItem.filter((detail) => !allDuplicate.includes(detail)), savedCount: 0 };
    return resolveAfterDelay(buildPreviewImportProgress());
  },
  continueImport: continuePreviewImport,
  cancelImport: () => Promise.resolve(),
  deleteImportFile: () => resolveAfterDelay(undefined),
  getStatus: () => resolveAfterDelay(previewStatus()),
  setAutoLock: (seconds: number) => {
    previewAutoLockSeconds = seconds;
    return resolveAfterDelay(previewStatus());
  },
  getTheme: () => Promise.resolve(previewTheme),
  setTheme: (appearance: string, color: string) => {
    previewTheme = { appearance, color };
    return Promise.resolve(previewTheme);
  },
  signIn: () => {
    previewState = "unlocked";
    return resolveAfterDelay(previewStatus());
  },
  signUp: (): Promise<SignUpResult> => {
    previewState = "unlocked";
    return resolveAfterDelay({ status: previewStatus(), secretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M" });
  },
  unlock: () => {
    previewState = "unlocked";
    return resolveAfterDelay(previewStatus());
  },
  lock: () => {
    previewState = "locked";
    return resolveAfterDelay(previewStatus());
  },
  signOut: () => {
    previewState = "signed-out";
    return resolveAfterDelay(previewStatus());
  },
  reportActivity: () => Promise.resolve(),
  listAllVault: () =>
    resolveAfterDelay(allPreviewVault.map((vault) => ({ ...vault, itemCount: allPreviewItemByVaultID[vault.id]?.length ?? 0 }))),
  createVault: (name: string) => {
    const vault = { id: name.toLowerCase(), name, type: "shared", role: "owner", itemCount: 0 };
    allPreviewVault.push(vault);
    allPreviewItemByVaultID[vault.id] = [];
    allPreviewAccessByVaultID[vault.id] = [{ accountId: "alper", role: "owner" }];
    return resolveAfterDelay(vault);
  },
  getVaultActions: (vaultID: string) => resolveAfterDelay(buildPreviewVaultActions(vaultID)),
  renameVault: (vaultID: string, name: string) => {
    const vault = allPreviewVault.find((candidate) => candidate.id === vaultID)!;
    vault.name = name.trim();
    return resolveAfterDelay(buildPreviewVaultSummary(vault));
  },
  deleteVault: (vaultID: string, request: VaultDeletionRequest): Promise<VaultDeletionResult> => {
    const vault = allPreviewVault.find((candidate) => candidate.id === vaultID)!;
    if (request.isDeletingItems && request.confirmedName !== vault.name) {
      return Promise.reject(new Error("type the vault's name to delete it together with its items"));
    }
    const allMovedItem = request.moveToVaultId ? (allPreviewItemByVaultID[vaultID] ?? []) : [];
    const target = allPreviewVault.find((candidate) => candidate.id === request.moveToVaultId);
    if (target) {
      allPreviewItemByVaultID[target.id].push(...allMovedItem);
    }
    removePreviewVault(vaultID);
    return resolveAfterDelay({ movedCount: allMovedItem.length, targetVaultName: target?.name ?? "" });
  },
  leaveVault: (vaultID: string) => {
    removePreviewVault(vaultID);
    return resolveAfterDelay(undefined);
  },
  listAllRemovedVault: () => resolveAfterDelay(takeAllPreviewRemovedVault()),
  getExportOptions: (): Promise<ExportOptions> =>
    resolveAfterDelay({ isBiometricAvailable: previewParameter.get("noTouchID") !== "1", minPassphraseLength: 10 }),
  exportAllData: exportPreviewData,
  listAllItem: (vaultID: string) =>
    resolveAfterDelay((allPreviewItemByVaultID[vaultID] ?? []).map((vaultItem) => markPreviewFavorite({ ...vaultItem, password: "", notes: "", fields: [] }))),
  getItem: (vaultID: string, itemID: string) =>
    resolveAfterDelay(markPreviewFavorite(structuredClone(allPreviewItemByVaultID[vaultID].find((vaultItem) => vaultItem.id === itemID)!))),
  listAllFavoriteItem: (): Promise<FavoriteItem[]> =>
    resolveAfterDelay(
      allPreviewVault
        .flatMap((vault) =>
          (allPreviewItemByVaultID[vault.id] ?? [])
            .filter((vaultItem) => allPreviewFavoriteItemID.has(vaultItem.id))
            .map((vaultItem) => ({ vaultId: vault.id, vaultName: vault.name, vaultType: vault.type, item: markPreviewFavorite({ ...vaultItem, password: "", notes: "", fields: [] }) })),
        )
        .sort((first, second) => first.item.title.localeCompare(second.item.title, undefined, { sensitivity: "base" })),
    ),
  setItemFavorite: (_vaultID: string, itemID: string, isFavorite: boolean) => {
    if (isFavorite) {
      allPreviewFavoriteItemID.add(itemID);
    } else {
      allPreviewFavoriteItemID.delete(itemID);
    }
    return resolveAfterDelay(undefined);
  },
  saveItem: (vaultID: string, detail: ItemDetail) => {
    const saved = { ...detail, id: detail.id || `item-${Date.now()}` };
    const allItem = allPreviewItemByVaultID[vaultID];
    const index = allItem.findIndex((vaultItem) => vaultItem.id === saved.id);
    if (index >= 0) {
      allItem[index] = saved;
    } else {
      allItem.push(saved);
    }
    return resolveAfterDelay(saved);
  },
  deletePasskey: (vaultID: string, itemID: string, credentialID: string) => {
    const vaultItem = allPreviewItemByVaultID[vaultID].find((candidate) => candidate.id === itemID)!;
    vaultItem.passkeys = vaultItem.passkeys.filter((passkey) => passkey.credentialId !== credentialID);
    return resolveAfterDelay(structuredClone(vaultItem));
  },
  // A rough stand-in for core/item's checks, enough to show the editor's brand badge and warnings.
  describeCard: (number: string, expiry: string): Promise<CardDescription> => {
    const digits = /^[0-9 -]+$/.test(number.trim()) ? number.replace(/[^0-9]/g, "") : "";
    const brand = digits.startsWith("4") ? "Visa" : /^5[1-5]/.test(digits) ? "Mastercard" : /^3[47]/.test(digits) ? "American Express" : /^9792/.test(digits) ? "Troy" : "";
    const sum = [...digits].reverse().reduce((total, character, index) => {
      const digit = Number(character) * (index % 2 === 1 ? 2 : 1);
      return total + (digit > 9 ? digit - 9 : digit);
    }, 0);
    const isValidNumber = digits.length >= 12 && digits.length <= 19 && sum % 10 === 0;
    return resolveAfterDelay({
      brand,
      last4: digits.length >= 4 ? digits.slice(-4) : "",
      numberWarning: number.trim() && !isValidNumber ? "This doesn't look like a valid card number" : "",
      expiryWarning: expiry.trim() && !/^(0?[1-9]|1[0-2])\s*[/.-]\s*\d{2}$/.test(expiry.trim()) ? "Use month and year, like 12/27" : "",
    });
  },
  convertToSecureNote: (vaultID: string, itemID: string) => {
    const vaultItem = allPreviewItemByVaultID[vaultID].find((candidate) => candidate.id === itemID)!;
    Object.assign(vaultItem, { category: "secure-note", canConvertToSecureNote: false });
    return resolveAfterDelay(structuredClone(vaultItem));
  },
  deleteItem: (vaultID: string, itemID: string) => {
    allPreviewItemByVaultID[vaultID] = allPreviewItemByVaultID[vaultID].filter((vaultItem) => vaultItem.id !== itemID);
    return resolveAfterDelay(undefined);
  },
  generatePassword: (config: PasswordConfig) => {
    const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ" + (config.digits ? "23456789" : "") + (config.symbols ? "!@#$%^&*-_=+?" : "");
    const allRandom = crypto.getRandomValues(new Uint32Array(config.length || 24));
    return resolveAfterDelay(Array.from(allRandom, (random) => alphabet[random % alphabet.length]).join(""));
  },
  generateSSHKey: (vaultID: string, title: string, keyType: SSHKeyType) => {
    if (!title.trim()) {
      return Promise.reject(new Error("enter a title"));
    }
    const detail = createPreviewSSHKey(`ssh-${Date.now()}`, title.trim(), keyType === "rsa" ? "ssh-rsa" : "ssh-ed25519");
    return resolveAfterDelay(structuredClone(addPreviewItem(vaultID, detail)));
  },
  importSSHKey: (vaultID: string, title: string, privateKeyText: string, passphrase: string) => {
    if (!privateKeyText.includes("PRIVATE KEY")) {
      return Promise.reject(new Error("this is not a private key in OpenSSH or PEM format"));
    }
    if (privateKeyText.includes("ENCRYPTED") && !passphrase) {
      return Promise.reject(new Error("this key is protected by a passphrase; enter it to import the key"));
    }
    const detail = createPreviewSSHKey(`ssh-${Date.now()}`, title.trim(), "ssh-ed25519");
    return resolveAfterDelay(structuredClone(addPreviewItem(vaultID, detail)));
  },
  listAllPerson: () => resolveAfterDelay(structuredClone(listAllPreviewPerson())),
  listAllVaultMember: (vaultID: string) => resolveAfterDelay(listAllPreviewVaultMember(vaultID)),
  shareVault: (vaultID: string, email: string, role: VaultRole) => {
    const normalizedEmail = email.trim().toLowerCase();
    if (!/^[^@\s]+@[^@\s]+$/.test(normalizedEmail)) {
      return Promise.reject(new Error("enter the email address of the person"));
    }
    if (vaultID === "personal") {
      return Promise.reject(new Error("personal vaults are private and cannot be shared; create a shared vault instead"));
    }
    if (listAllPreviewVaultMember(vaultID).some((member) => member.email === normalizedEmail)) {
      return Promise.reject(new Error("this person already has access to the vault"));
    }
    const vaultName = allPreviewVault.find((vault) => vault.id === vaultID)?.name ?? "";
    previewShareInvitations.sent = [
      ...previewShareInvitations.sent.filter((invitation) => !(invitation.vaultId === vaultID && invitation.email === normalizedEmail)),
      {
        id: `to-${Date.now()}`,
        vaultId: vaultID,
        vaultName,
        email: normalizedEmail,
        recipientName: "",
        role,
        state: "pending",
        expiresAt: new Date(Date.now() + 7 * 24 * hourInMilliseconds).toISOString(),
        acceptedAt: "",
      },
    ];
    return resolveAfterDelay(undefined);
  },
  listAllShareInvitation: () => resolveAfterDelay(structuredClone(previewShareInvitations)),
  acceptShareInvitation: (invitationID: string) => {
    const accepted = previewShareInvitations.received.find((invitation) => invitation.id === invitationID);
    previewShareInvitations.received = previewShareInvitations.received.filter((invitation) => invitation.id !== invitationID);
    if (accepted) {
      const { id, vaultId, role, invitedByName, invitedByEmail } = accepted;
      previewShareInvitations.waiting = [...(previewShareInvitations.waiting ?? []), { id, vaultId, role, invitedByName, invitedByEmail, acceptedAt: new Date().toISOString() }];
    }
    return resolveAfterDelay(undefined);
  },
  listAllShareAwaitingUnlock: () =>
    resolveAfterDelay(
      previewState === "locked" && previewParameter.get("shareInvitations") !== "none"
        ? [{ invitationId: "to-ops", recipientName: "Ada Kaya", recipientEmail: "ops@kaya.dev", acceptedAt: new Date(Date.now() - 2 * hourInMilliseconds).toISOString() }]
        : [],
    ),
  declineShareInvitation: (invitationID: string) => {
    previewShareInvitations.received = previewShareInvitations.received.filter((invitation) => invitation.id !== invitationID);
    return resolveAfterDelay(undefined);
  },
  cancelShareInvitation: (invitationID: string) => {
    previewShareInvitations.sent = previewShareInvitations.sent.filter((invitation) => invitation.id !== invitationID);
    return resolveAfterDelay(undefined);
  },
  listAllOrganization: () => resolveAfterDelay({ organizations: listAllPreviewOrganizationSummary(), invitations: structuredClone(allPreviewOrganizationInvitation) }),
  getOrganization: (organizationID: string) =>
    allPreviewOrganization.some((organization) => organization.id === organizationID)
      ? resolveAfterDelay(buildPreviewOrganization(organizationID))
      : Promise.reject(new Error("that organization, team, person or vault was not found, or you are no longer in the organization")),
  createOrganization: (name: string): Promise<OrganizationSummary> => {
    const cleanName = name.trim();
    if (cleanName === "" || cleanName.length > 100) {
      return Promise.reject(new Error("enter a name of 1 to 100 characters"));
    }
    const id = `organization-${Date.now()}`;
    allPreviewOrganization.push({ id, name: cleanName, role: "admin", isAdmin: true, members: [{ ...findPreviewPerson("alper"), role: "admin" }], teams: [], invitations: [] });
    return resolveAfterDelay({ id, name: cleanName, role: "admin", memberCount: 1 });
  },
  renameOrganization: (organizationID: string, name: string) => changePreviewOrganization(() => (findPreviewOrganization(organizationID).name = name.trim())),
  deleteOrganization: (organizationID: string) => {
    if (allPreviewVault.some((vault) => vault.organizationId === organizationID) || findPreviewOrganization(organizationID).teams.some((team) => team.vaults.length > 0)) {
      return Promise.reject(new Error("the organization still has vaults; delete them first"));
    }
    allPreviewOrganization.splice(allPreviewOrganization.indexOf(findPreviewOrganization(organizationID)), 1);
    return resolveAfterDelay(undefined);
  },
  leaveOrganization: (organizationID: string) => {
    const organization = findPreviewOrganization(organizationID);
    if (organization.members.length === 1) {
      return Promise.reject(new Error("you are the organization's only member; delete the organization instead"));
    }
    if (organization.isAdmin && organization.members.filter((member) => member.role === "admin").length === 1) {
      return Promise.reject(new Error("an organization needs an admin; make someone else an admin first"));
    }
    allPreviewOrganization.splice(allPreviewOrganization.indexOf(organization), 1);
    for (const vault of allPreviewVault.filter((candidate) => candidate.organizationId === organizationID)) {
      removePreviewVault(vault.id);
    }
    return resolveAfterDelay(undefined);
  },
  inviteToOrganization: (organizationID: string, email: string, role: OrganizationRole) => {
    const normalizedEmail = email.trim().toLowerCase();
    if (!/^[^@\s]+@[^@\s]+$/.test(normalizedEmail)) {
      return Promise.reject(new Error("enter the email address of the person"));
    }
    const organization = findPreviewOrganization(organizationID);
    if (normalizedEmail === "alper@tryhisar.com") {
      return Promise.reject(new Error("that is your own email; you are already in the organization"));
    }
    if (organization.members.some((member) => member.email === normalizedEmail)) {
      return Promise.reject(new Error("this person is already a member of the organization"));
    }
    organization.invitations = [
      ...organization.invitations.filter((invitation) => invitation.email !== normalizedEmail),
      { id: `to-${Date.now()}`, email: normalizedEmail, role, createdAt: new Date().toISOString(), expiresAt: new Date(Date.now() + 7 * 24 * hourInMilliseconds).toISOString() },
    ];
    return resolveAfterDelay(undefined);
  },
  cancelOrganizationInvitation: (organizationID: string, invitationID: string) => {
    const organization = findPreviewOrganization(organizationID);
    organization.invitations = organization.invitations.filter((invitation) => invitation.id !== invitationID);
    return resolveAfterDelay(undefined);
  },
  acceptOrganizationInvitation: (invitationID: string) => {
    const invitation = allPreviewOrganizationInvitation.find((candidate) => candidate.id === invitationID)!;
    allPreviewOrganizationInvitation = allPreviewOrganizationInvitation.filter((candidate) => candidate.id !== invitationID);
    allPreviewOrganization.push({
      id: invitation.organizationId,
      name: invitation.organizationName,
      role: invitation.role,
      isAdmin: invitation.role === "admin",
      members: [
        { ...findPreviewPerson("alper"), role: invitation.role },
        { accountId: "ada", name: invitation.invitedByName, email: invitation.invitedByEmail, role: "admin", securityCode: "5C1E 0B7A 93F2 D468", isCurrentUser: false, isKeyChanged: false },
      ],
      teams: [],
      invitations: [],
    });
    return resolveAfterDelay(undefined);
  },
  declineOrganizationInvitation: (invitationID: string) => {
    allPreviewOrganizationInvitation = allPreviewOrganizationInvitation.filter((invitation) => invitation.id !== invitationID);
    return resolveAfterDelay(undefined);
  },
  setOrganizationMemberRole: (organizationID: string, accountID: string, role: OrganizationRole) => {
    const organization = findPreviewOrganization(organizationID);
    if (role === "member" && organization.members.filter((member) => member.role === "admin" && member.accountId !== accountID).length === 0) {
      return Promise.reject(new Error("an organization needs an admin; make someone else an admin first"));
    }
    organization.members.find((member) => member.accountId === accountID)!.role = role;
    if (accountID === "alper") {
      Object.assign(organization, { role, isAdmin: role === "admin", invitations: role === "admin" ? organization.invitations : [] });
    }
    return resolveAfterDelay(undefined);
  },
  removeOrganizationMember: (organizationID: string, accountID: string) =>
    changePreviewOrganization(() => {
      const organization = findPreviewOrganization(organizationID);
      organization.members = organization.members.filter((member) => member.accountId !== accountID);
      for (const team of organization.teams) {
        team.members = team.members.filter((member) => member.accountId !== accountID);
      }
    }),
  createTeam: (organizationID: string, name: string): Promise<Team> => {
    const organization = findPreviewOrganization(organizationID);
    if (organization.teams.some((team) => team.name.toLowerCase() === name.trim().toLowerCase())) {
      return Promise.reject(new Error("the organization already has a team with this name"));
    }
    const team: Team = { id: `team-${Date.now()}`, name: name.trim(), members: [], vaults: [] };
    organization.teams = [...organization.teams, team].sort((first, second) => first.name.localeCompare(second.name));
    return resolveAfterDelay(structuredClone(team));
  },
  renameTeam: (organizationID: string, teamID: string, name: string) => changePreviewOrganization(() => (findPreviewTeam(organizationID, teamID).name = name.trim())),
  deleteTeam: (organizationID: string, teamID: string) =>
    changePreviewOrganization(() => {
      const organization = findPreviewOrganization(organizationID);
      organization.teams = organization.teams.filter((team) => team.id !== teamID);
    }),
  addTeamMember: (organizationID: string, teamID: string, accountID: string) => {
    const { name, email, isCurrentUser } = findPreviewOrganization(organizationID).members.find((member) => member.accountId === accountID)!;
    findPreviewTeam(organizationID, teamID).members.push({ accountId: accountID, name, email, isCurrentUser });
    return resolveAfterDelay(undefined);
  },
  removeTeamMember: (organizationID: string, teamID: string, accountID: string) =>
    changePreviewOrganization(() => {
      const team = findPreviewTeam(organizationID, teamID);
      team.members = team.members.filter((member) => member.accountId !== accountID);
    }),
  giveVaultToTeam: (organizationID: string, teamID: string, vaultID: string, role: VaultRole) =>
    changePreviewOrganization(() => {
      const team = findPreviewTeam(organizationID, teamID);
      team.vaults = [...team.vaults.filter((teamVault) => teamVault.vaultId !== vaultID), { vaultId: vaultID, name: "", role, canOpen: false }];
    }),
  takeVaultFromTeam: (organizationID: string, teamID: string, vaultID: string) =>
    changePreviewOrganization(() => {
      const team = findPreviewTeam(organizationID, teamID);
      team.vaults = team.vaults.filter((teamVault) => teamVault.vaultId !== vaultID);
    }),
  createOrganizationVault: (organizationID: string, teamID: string, name: string): Promise<VaultSummary> => {
    const vault = { id: `${name.trim().toLowerCase().replace(/\s+/g, "-")}-${Date.now()}`, name: name.trim(), type: "organization", role: "owner", itemCount: 0, organizationId: organizationID };
    allPreviewVault.push(vault);
    allPreviewItemByVaultID[vault.id] = [];
    findPreviewTeam(organizationID, teamID).vaults.push({ vaultId: vault.id, name: vault.name, role: "owner", canOpen: true });
    return resolveAfterDelay(vault);
  },
  listAllWaitingVaultKey: () => resolveAfterDelay(structuredClone(previewWaitingVaultKeys)),
  changeVaultMemberRole: (vaultID: string, accountID: string, role: VaultRole) => {
    const access = findPreviewAccess(vaultID, accountID);
    if (access) {
      access.role = role;
    }
    return resolveAfterDelay(undefined);
  },
  removeVaultMember: (vaultID: string, accountID: string) => {
    allPreviewAccessByVaultID[vaultID] = (allPreviewAccessByVaultID[vaultID] ?? []).filter((access) => access.accountId !== accountID);
    return new Promise<void>((resolve) => setTimeout(resolve, 600));
  },
  invitePerson: (email: string) => {
    if (!email.includes("@")) {
      return Promise.reject(new Error("enter the email address of the person"));
    }
    return resolveAfterDelay(undefined);
  },
  listAllDevice: () => resolveAfterDelay(structuredClone(allPreviewDevice)),
  revokeDevice: (deviceID: string) => {
    allPreviewDevice = allPreviewDevice.filter((device) => device.id !== deviceID);
    return resolveAfterDelay(undefined);
  },
  getSSHAgentStatus: () => resolveAfterDelay(previewAgentStatus()),
  configureSSHConfig: () => {
    isPreviewSSHConfigured = true;
    return resolveAfterDelay(previewAgentStatus("/Users/alper/.ssh/config.hisar-backup-20261008-093000"));
  },
  removeSSHConfig: () => {
    isPreviewSSHConfigured = false;
    return resolveAfterDelay(previewAgentStatus("/Users/alper/.ssh/config.hisar-backup-20261008-093100"));
  },
  respondSSHApproval: (requestID: string, isAuthorized: boolean) => {
    if (isAuthorized && previewParameter.get("approvalCompletes") === "1") {
      setTimeout(() => import("./approval").then(({ removeApproval }) => removeApproval(requestID)), 900);
    }
    return resolveAfterDelay(undefined);
  },
  listAllPendingSSHApproval: () => resolveAfterDelay(listAllPreviewApproval()),
  respondApprovalWithPassword: (requestID: string, password: string) => {
    if (password !== "correct horse battery") {
      setTimeout(() => import("./approval").then(({ failApproval }) => failApproval(requestID, "Incorrect master password.")), 400);
    } else {
      setTimeout(() => import("./approval").then(({ removeApproval }) => removeApproval(requestID)), 600);
    }
    return resolveAfterDelay(undefined);
  },
  getCommandLineStatus: () => resolveAfterDelay({ ...previewCommandLineStatus }),
  listAllAgentVaultID: () => resolveAfterDelay([...allPreviewAgentVaultID]),
  setVaultAvailableToAgents: (vaultID: string, isAvailable: boolean) => {
    allPreviewAgentVaultID = allPreviewAgentVaultID.filter((candidate) => candidate !== vaultID);
    if (isAvailable) {
      allPreviewAgentVaultID.push(vaultID);
    }
    return resolveAfterDelay([...allPreviewAgentVaultID]);
  },
  installCommandLineTool: () => {
    if (previewCommandLineMode === "conflict") {
      return Promise.reject(new Error("/opt/homebrew/bin/hisar is already a file that is not from Hisar; Hisar leaves it alone. Remove it yourself to install Hisar's"));
    }
    isPreviewCommandLineInstalled = true;
    previewCommandLineStatus = buildPreviewCommandLineStatus(true);
    return resolveAfterDelay({ ...previewCommandLineStatus });
  },
  uninstallCommandLineTool: () => {
    isPreviewCommandLineInstalled = false;
    previewCommandLineStatus = buildPreviewCommandLineStatus(false);
    return resolveAfterDelay({ ...previewCommandLineStatus });
  },
  setCommandLineIntegration: (isEnabled: boolean) => {
    previewCommandLineStatus = { ...buildPreviewCommandLineStatus(isPreviewCommandLineInstalled), isIntegrationEnabled: isEnabled, isServing: isEnabled };
    return resolveAfterDelay({ ...previewCommandLineStatus });
  },
  copyField: () => resolveAfterDelay(undefined),
  getOneTimeCode: () => {
    const period = 30;
    const counter = Math.floor(Date.now() / 1000 / period);
    const code = String((counter * 7919 + 482913) % 1_000_000).padStart(6, "0");
    return resolveAfterDelay({ code, expiresAt: new Date((counter + 1) * period * 1000).toISOString(), period });
  },
  copyText: () => resolveAfterDelay(undefined),
  listAllShareLinkField: (vaultID: string, itemID: string) =>
    resolveAfterDelay(listAllPreviewShareLinkField(allPreviewItemByVaultID[vaultID].find((vaultItem) => vaultItem.id === itemID)!)),
  createShareLink: (request: ShareLinkRequest) => {
    const created = createPreviewShareLinkFromRequest(request);
    allPreviewShareLink = [created, ...allPreviewShareLink];
    return resolveAfterDelay(structuredClone(created));
  },
  listAllShareLink: () => resolveAfterDelay(allPreviewShareLink.map((shareLink) => ({ ...shareLink, url: "" }))),
  revokeShareLink: (shareLinkID: string) => {
    allPreviewShareLink = allPreviewShareLink.filter((shareLink) => shareLink.id !== shareLinkID);
    return resolveAfterDelay(undefined);
  },
  copyShareLink: () => resolveAfterDelay(undefined),
  copySecretReference: (vaultID: string, itemID: string, fieldName: string) => {
    const encodeSegment = (segment: string) => segment.replace(/[^A-Za-z0-9\-._~\u0080-\uffff]/g, (character) => `%${character.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0")}`);
    const vaultName = allPreviewVault.find((vault) => vault.id === vaultID)?.name ?? vaultID;
    const itemTitle = allPreviewItemByVaultID[vaultID]?.find((detail) => detail.id === itemID)?.title ?? itemID;
    const field = fieldName === "one-time code" ? "otp" : fieldName;
    return resolveAfterDelay(`hisar://${encodeSegment(vaultName)}/${encodeSegment(itemTitle)}/${encodeSegment(field)}`);
  },
  unlockWithBiometric: () =>
    new Promise<Status>((resolve, reject) => {
      if (new URLSearchParams(window.location.search).get("touchIdWait") === "1") {
        return;
      }
      setTimeout(() => {
        if (previewState !== "locked") {
          reject(new Error("Touch ID was cancelled"));
          return;
        }
        previewState = "unlocked";
        resolve(previewStatus());
      }, 1500);
    }),
  enableBiometricUnlock: (password: string) => {
    if (password.length < 4) {
      return Promise.reject(new Error("incorrect email, master password or Secret Key"));
    }
    isPreviewBiometricEnabled = true;
    return resolveAfterDelay(previewStatus());
  },
  enableNewAccountBiometricUnlock: () => {
    isPreviewBiometricEnabled = true;
    return resolveAfterDelay(previewStatus());
  },
  disableBiometricUnlock: () => {
    isPreviewBiometricEnabled = false;
    return resolveAfterDelay(previewStatus());
  },
  setTouchIDPromptFrame: () => Promise.resolve(),
  clearTouchIDPromptFrame: () => Promise.resolve(),
  setSSHApprovalRemembered: () => Promise.resolve(),
  setCompactSSHApproval: (isCompact: boolean) => {
    isPreviewCompactApproval = isCompact;
    return resolveAfterDelay(previewAgentStatus());
  },
  setPhoneApproval: (isEnabled: boolean) => {
    isPreviewPhoneApproval = isEnabled;
    return resolveAfterDelay(previewAgentStatus());
  },
  setSigningRemember: (seconds: number) => {
    previewSigningRememberSeconds = seconds;
    return resolveAfterDelay(previewAgentStatus());
  },
  setSSHRemember: (seconds: number) => {
    previewSSHRememberSeconds = seconds;
    return resolveAfterDelay(previewAgentStatus());
  },
  fitSSHApprovalWindow: () => Promise.resolve(),
  setApprovalTouchIDFrame: () => Promise.resolve(),
  clearApprovalTouchIDFrame: () => Promise.resolve(),
  cancelApprovalTouchID: () => Promise.resolve(),
  cancelTouchIDPrompt: () => Promise.resolve(),
  hasNativeTouchIDView: false,
  saveNewAccountEmergencyKit: () => resolveAfterDelay("/Users/you/Documents/Hisar Emergency Kit.pdf"),
  saveEmergencyKit: (password: string) =>
    password.length < 4 ? Promise.reject(new Error("incorrect email, master password or Secret Key")) : resolveAfterDelay("/Users/you/Documents/Hisar Emergency Kit.pdf"),
  forgetNewAccountSecretKey: () => Promise.resolve(),
  changeMasterPassword: (currentPassword: string) =>
    currentPassword.length < 4
      ? Promise.reject(new Error("the master password is not correct"))
      : resolveAfterDelay({ status: previewStatus(), secretKey: "", signedOutDeviceCount: 2, keychainError: "" }),
  replaceSecretKey: (password: string) =>
    password.length < 4
      ? Promise.reject(new Error("the master password is not correct"))
      : resolveAfterDelay({ status: previewStatus(), secretKey: "H1-GVC2KA-8RW2N-QHT4M-0ZKXD-71BJE-P3SV6A", signedOutDeviceCount: 2, keychainError: "" }),
  previewAccountDeletion: () =>
    resolveAfterDelay({
      email: "alper@tryhisar.com",
      ownedVaults: [
        { id: "personal", name: "Personal", type: "personal", role: "owner", otherMemberCount: 0 },
        { id: "family", name: "Family", type: "shared", role: "owner", otherMemberCount: 2 },
      ],
      memberVaults: [
        { id: "infra", name: "Infrastructure", type: "shared", role: "owner", otherMemberCount: 0 },
        { id: "design", name: "Design team", type: "shared", role: "editor", otherMemberCount: 0 },
      ],
      shareLinkCount: 3,
      sentInvitationCount: 1,
      receivedInvitationCount: 0,
      deviceCount: 3,
      organizations: hasPreviewOrganizations
        ? [
            { id: "acme", name: "Acme", isDeleted: false, newAdminName: "Elif Yılmaz" },
            { id: "globex", name: "Globex", isDeleted: false, newAdminName: "" },
            { id: "side-project", name: "Side project", isDeleted: true, newAdminName: "" },
          ]
        : [],
    }),
  deleteAccount: (password: string) => {
    if (password.length < 4) {
      return Promise.reject(new Error("the master password is not correct"));
    }
    previewState = "signed-out";
    return resolveAfterDelay(previewStatus());
  },
  dismissSignedOutReason: () => {
    previewSignedOutReason = "";
    return resolveAfterDelay(previewStatus());
  },
  getBrowserStatus: () => resolveAfterDelay({ ...previewBrowserStatus, allPairedBrowser: [...previewBrowserStatus.allPairedBrowser] }),
  removePairedBrowser: (instanceID: string) => {
    previewBrowserStatus.allPairedBrowser = previewBrowserStatus.allPairedBrowser.filter((pairedBrowser) => pairedBrowser.instanceID !== instanceID);
    return resolveAfterDelay({ ...previewBrowserStatus });
  },
  setExactHostMatch: (isExactHostMatch: boolean) => {
    previewBrowserStatus.isExactHostMatch = isExactHostMatch;
    return resolveAfterDelay({ ...previewBrowserStatus });
  },
  checkMinimumVersion: () => resolveAfterDelay(previewStatus()),
  takePendingInvite: () => Promise.resolve(previewParameter.get("invite") ?? ""),
  getUpdateStatus: () => resolveAfterDelay({ ...previewUpdateStatus }),
  checkForUpdates: () => {
    if (!previewUpdateStatus.isSignedIn) {
      return Promise.reject(new Error("sign in to check for updates"));
    }
    return resolveAfterDelay({ ...previewUpdateStatus });
  },
  setAutomaticUpdateCheck: (isEnabled: boolean) => {
    previewUpdateStatus.isAutomaticCheckEnabled = isEnabled;
    return resolveAfterDelay({ ...previewUpdateStatus });
  },
  getLoginItemStatus: () => resolveAfterDelay({ ...previewLoginItemStatus }),
  setOpenAtLogin: (isEnabled: boolean) => {
    previewLoginItemStatus.isOpenAtLogin = isEnabled;
    previewLoginItemStatus.systemState = isEnabled ? "enabled" : "not-registered";
    return resolveAfterDelay({ ...previewLoginItemStatus });
  },
  openLoginItemSettings: () => Promise.resolve(),
  cancelBrowserPairing: () => {
    previewBrowserStatus.pendingPairing = null;
    return Promise.resolve();
  },
  openURL: (link: string) => {
    window.open(link, "_blank", "noopener");
    return Promise.resolve();
  },
};
