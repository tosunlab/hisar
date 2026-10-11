// Development-only stand-in for the Go backend, used with `npm run preview:ui` to view the UI in a browser.
import type {
  ApprovalEvent,
  BrowserStatus,
  CommandLineStatus,
  Device,
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
} from "./api";

const previewParameter = new URLSearchParams(window.location.search);

const allPreviewVault: VaultSummary[] = [
  { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0 },
  { id: "family", name: "Family", type: "shared", role: "owner", itemCount: 0 },
  { id: "work", name: "Work", type: "shared", role: "reader", itemCount: 0 },
];

function createPreviewItem(id: string, title: string, username: string, link: string, password: string, extra: Partial<ItemDetail> = {}): ItemDetail {
  return {
    id,
    revision: "rev",
    category: "",
    title,
    username,
    urls: link ? [link] : [],
    password,
    notes: "",
    fields: [],
    publicKey: "",
    fingerprint: "",
    keyType: "",
    oneTimePassword: "",
    hasOneTimePassword: false,
    passkeys: [],
    ...extra,
  };
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
  ],
  family: [
    createPreviewItem("netflix", "Netflix", "family@example.com", "https://netflix.com", "netflix-family-pass", { notes: "Shared family account" }),
    createPreviewItem("wifi", "Home Wi-Fi", "", "", "wifi-password-123", { fields: [{ name: "Network", value: "Hisar-Home", concealed: false }] }),
    createPreviewItem("spotify", "Spotify Family", "family@example.com", "https://spotify.com", "Tg5!kP9#vL2@"),
    createPreviewSSHKey("home-server", "Home server", "ssh-ed25519"),
  ],
  work: [createPreviewItem("vpn", "Office VPN", "alper", "https://vpn.example.com", "Vq8@Lm3!Zr6#")],
};

function getInitialState(): string {
  return new URLSearchParams(window.location.search).get("state") ?? "unlocked";
}

let previewState = getInitialState();
let previewSignedOutEmail = "alper@tryhisar.com";
let isPreviewBiometricEnabled = new URLSearchParams(window.location.search).get("touchId") === "1";
let previewAutoLockSeconds = 600;
// &appearance=light|dark|system and &color=neutral|blue|purple|green pick the theme the preview starts with.
let previewTheme: Theme = { appearance: previewParameter.get("appearance") ?? "system", color: previewParameter.get("color") ?? "neutral" };
const previewStatus = (): Status => ({
  state: previewState,
  server: "https://tryhisar.com",
  email: previewState === "signed-out" ? previewSignedOutEmail : "alper@tryhisar.com",
  name: "Alper",
  biometricAvailable: true,
  biometricEnabled: isPreviewBiometricEnabled,
  biometricReady: isPreviewBiometricEnabled && previewState === "locked",
  isOffline: new URLSearchParams(window.location.search).get("offline") === "1",
  autoLockSeconds: previewAutoLockSeconds,
});
let isPreviewSSHConfigured = new URLSearchParams(window.location.search).get("sshConfigured") === "1";
let isPreviewCompactApproval = new URLSearchParams(window.location.search).get("compact") === "1";
let isPreviewPhoneApproval = new URLSearchParams(window.location.search).get("phoneApprovalOff") !== "1";
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
  family: [
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
    ? { received: [], sent: [], completionError: "" }
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
            vaultId: "family",
            vaultName: "Family",
            email: "zeynep@tryhisar.com",
            role: "reader",
            state: "pending",
            expiresAt: new Date(Date.now() + 5 * 24 * hourInMilliseconds).toISOString(),
          },
          {
            id: "to-ops",
            vaultId: "family",
            vaultName: "Family",
            email: "ops@kaya.dev",
            role: "editor",
            state: "accepted",
            expiresAt: new Date(Date.now() + 2 * 24 * hourInMilliseconds).toISOString(),
          },
        ],
        completionError: "",
      };

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
        isRememberable: false,
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
  createPreviewItem("import-netflix", "Netflix", "family@example.com", "https://netflix.com", "imported-4"),
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
  createPreviewItem("import-cabin", "Wi-Fi at the cabin", "", "", "", { notes: "Network: Kale\nKey: under the mat" }),
  createPreviewItem("import-codes", "Server recovery codes", "", "", "", { notes: "1. 4821-9930\n2. 1177-0042" }),
  createPreviewItem("import-insurance", "Car insurance", "", "", "", { notes: "Policy 88-120-4471, renews in March" }),
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
    loginCount: allPreviewImportItem.filter((detail) => detail.category === "" && (detail.username || detail.password || detail.urls.length > 0)).length,
    secureNoteCount: allPreviewImportItem.filter((detail) => detail.category === "" && !detail.username && !detail.password && detail.urls.length === 0).length,
    sshKeyCount: allPreviewImportItem.filter((detail) => detail.category === "ssh-key").length,
    allSkipped: allPreviewImportSkipped,
    allDuplicate: findAllPreviewImportDuplicate(vaultID).map((detail) => ({ title: detail.title, username: detail.username, url: detail.urls[0] ?? "" })),
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
  listAllItem: (vaultID: string) =>
    resolveAfterDelay((allPreviewItemByVaultID[vaultID] ?? []).map((vaultItem) => ({ ...vaultItem, password: "", notes: "", fields: [] }))),
  getItem: (vaultID: string, itemID: string) =>
    resolveAfterDelay(structuredClone(allPreviewItemByVaultID[vaultID].find((vaultItem) => vaultItem.id === itemID)!)),
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
      { id: `to-${Date.now()}`, vaultId: vaultID, vaultName, email: normalizedEmail, role, state: "pending", expiresAt: new Date(Date.now() + 7 * 24 * hourInMilliseconds).toISOString() },
    ];
    return resolveAfterDelay(undefined);
  },
  listAllShareInvitation: () => resolveAfterDelay(structuredClone(previewShareInvitations)),
  acceptShareInvitation: (invitationID: string) => {
    previewShareInvitations.received = previewShareInvitations.received.filter((invitation) => invitation.id !== invitationID);
    return resolveAfterDelay(undefined);
  },
  declineShareInvitation: (invitationID: string) => {
    previewShareInvitations.received = previewShareInvitations.received.filter((invitation) => invitation.id !== invitationID);
    return resolveAfterDelay(undefined);
  },
  cancelShareInvitation: (invitationID: string) => {
    previewShareInvitations.sent = previewShareInvitations.sent.filter((invitation) => invitation.id !== invitationID);
    return resolveAfterDelay(undefined);
  },
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
  getBrowserStatus: () => resolveAfterDelay({ ...previewBrowserStatus, allPairedBrowser: [...previewBrowserStatus.allPairedBrowser] }),
  removePairedBrowser: (instanceID: string) => {
    previewBrowserStatus.allPairedBrowser = previewBrowserStatus.allPairedBrowser.filter((pairedBrowser) => pairedBrowser.instanceID !== instanceID);
    return resolveAfterDelay({ ...previewBrowserStatus });
  },
  setExactHostMatch: (isExactHostMatch: boolean) => {
    previewBrowserStatus.isExactHostMatch = isExactHostMatch;
    return resolveAfterDelay({ ...previewBrowserStatus });
  },
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
