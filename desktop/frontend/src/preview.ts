// Development-only stand-in for the Go backend, used with `npm run preview:ui` to view the UI in a browser.
import type {
  Device,
  FamilyMember,
  ItemDetail,
  PasswordConfig,
  PendingRecovery,
  RecoverAccountResult,
  RecoveryStatus,
  SignUpResult,
  SSHAgentStatus,
  SSHKeyType,
  Status,
  VaultMember,
  VaultRole,
  VaultSummary,
} from "./api";

const previewParameter = new URLSearchParams(window.location.search);
// recovery=off: not set up; recovery=other: set up by Elif, so only she can complete recoveries.
const previewRecoveryMode = previewParameter.get("recovery") ?? "on";
const isPreviewAdmin = previewParameter.get("role") !== "member";
const isPreviewOptOut = previewParameter.get("optOut") === "1";

const allPreviewVault: VaultSummary[] = [
  {
    id: "personal",
    name: "Personal",
    type: "personal",
    role: "owner",
    itemCount: 0,
    recoverable: previewRecoveryMode !== "off" && !isPreviewOptOut,
    recoveryOptOut: isPreviewOptOut,
  },
  { id: "family", name: "Family", type: "shared", role: "owner", itemCount: 0, recoverable: previewRecoveryMode !== "off", recoveryOptOut: false },
  { id: "work", name: "Work", type: "shared", role: "reader", itemCount: 0, recoverable: previewRecoveryMode !== "off", recoveryOptOut: false },
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
      notes: "Recovery codes are in the safe.\nTwo-factor via the authenticator app.",
      fields: [{ name: "Recovery code", value: "a1b2-c3d4-e5f6", concealed: true }],
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
const previewStatus = (): Status => ({
  state: previewState,
  server: "https://tryhisar.com",
  email: previewState === "signed-out" ? previewSignedOutEmail : "alper@tryhisar.com",
  name: "Alper",
  biometricAvailable: true,
  biometricEnabled: isPreviewBiometricEnabled,
  biometricReady: isPreviewBiometricEnabled && previewState === "locked",
  isOffline: new URLSearchParams(window.location.search).get("offline") === "1",
});
let isPreviewSSHConfigured = new URLSearchParams(window.location.search).get("sshConfigured") === "1";
const previewAgentStatus = (backupPath = ""): SSHAgentStatus => {
  const hasError = new URLSearchParams(window.location.search).get("agent") === "error";
  return {
    socketPath: "/Users/alper/.hisar/agent.sock",
    isRunning: !hasError,
    errorMessage: hasError ? "sshagent: another agent is listening on /Users/alper/.hisar/agent.sock" : "",
    isSSHConfigured: isPreviewSSHConfigured,
    configPath: "/Users/alper/.ssh/config",
    configSection: '# Added by Hisar: use the Hisar SSH agent\nHost *\n  IdentityAgent "~/.hisar/agent.sock"\n# End of Hisar section\n',
    isTouchIDApproval: new URLSearchParams(window.location.search).get("approval") !== "1",
    backupPath,
  };
};

const allPreviewFamilyMember: FamilyMember[] = [
  { accountId: "alper", name: "Alper", email: "alper@tryhisar.com", role: isPreviewAdmin ? "admin" : "member", securityCode: "3F2A 91C0 7B44 E1D8", isCurrentUser: true, isKeyChanged: false },
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

const previewRecoveryStatus: RecoveryStatus = {
  isSetUp: previewRecoveryMode !== "off",
  isAdmin: isPreviewAdmin,
  isCreatedByCurrentUser: previewRecoveryMode === "on" && isPreviewAdmin,
  createdByName: previewRecoveryMode === "on" && isPreviewAdmin ? "Alper" : previewRecoveryMode === "off" ? "" : "Elif Yılmaz",
};

let allPreviewPendingRecovery: PendingRecovery[] = [
  {
    accountId: "deniz",
    name: "Deniz Aydın",
    email: "deniz@tryhisar.com",
    submittedAt: new Date(Date.now() - 3 * hourInMilliseconds).toISOString(),
    recoverableVaultCount: 2,
    unrecoverableVaultCount: 1,
  },
  {
    accountId: "murat",
    name: "Murat Kaya",
    email: "murat@tryhisar.com",
    submittedAt: new Date(Date.now() - 26 * hourInMilliseconds).toISOString(),
    recoverableVaultCount: 3,
    unrecoverableVaultCount: 0,
  },
];

function setPreviewVaultRecovery(vaultID: string, isRecoverable: boolean): VaultSummary {
  const vault = allPreviewVault.find((candidate) => candidate.id === vaultID)!;
  vault.recoverable = isRecoverable;
  vault.recoveryOptOut = !isRecoverable;
  return { ...vault, itemCount: allPreviewItemByVaultID[vaultID]?.length ?? 0 };
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
      const member = allPreviewFamilyMember.find((candidate) => candidate.accountId === access.accountId)!;
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

const resolveAfterDelay = <Value>(value: Value): Promise<Value> => new Promise((resolve) => setTimeout(() => resolve(value), 80));

export const previewApi = {
  getStatus: () => resolveAfterDelay(previewStatus()),
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
    const vault = { id: name.toLowerCase(), name, type: "shared", role: "owner", itemCount: 0, recoverable: previewRecoveryStatus.isSetUp, recoveryOptOut: false };
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
  listAllMember: () => resolveAfterDelay(structuredClone(allPreviewFamilyMember)),
  listAllVaultMember: (vaultID: string) => resolveAfterDelay(listAllPreviewVaultMember(vaultID)),
  shareVault: (vaultID: string, email: string, role: VaultRole) => {
    const member = allPreviewFamilyMember.find((candidate) => candidate.email === email);
    if (!member) {
      return Promise.reject(new Error("nobody in your family uses that email; invite them first in Settings"));
    }
    if (vaultID === "personal") {
      return Promise.reject(new Error("personal vaults are private and cannot be shared; create a shared vault instead"));
    }
    allPreviewAccessByVaultID[vaultID] = [...(allPreviewAccessByVaultID[vaultID] ?? [{ accountId: "alper", role: "owner" }]), { accountId: member.accountId, role }];
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
  inviteMember: (email: string) => {
    if (!email.includes("@")) {
      return Promise.reject(new Error("enter the email address of the person you are inviting"));
    }
    return resolveAfterDelay("https://tryhisar.com/invite/kq3Vx9TzR2mW8pLc4NbY7hJd");
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
  respondSSHApproval: () => resolveAfterDelay(undefined),
  copyField: () => resolveAfterDelay(undefined),
  copyText: () => resolveAfterDelay(undefined),
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
  cancelTouchIDPrompt: () => Promise.resolve(),
  hasNativeTouchIDView: false,
  saveNewAccountEmergencyKit: () => resolveAfterDelay("/Users/you/Documents/Hisar Emergency Kit.pdf"),
  saveEmergencyKit: (password: string) =>
    password.length < 4 ? Promise.reject(new Error("incorrect email, master password or Secret Key")) : resolveAfterDelay("/Users/you/Documents/Hisar Emergency Kit.pdf"),
  forgetNewAccountSecretKey: () => Promise.resolve(),
  getRecoveryStatus: () => resolveAfterDelay({ ...previewRecoveryStatus }),
  setUpRecovery: () => {
    previewRecoveryStatus.isSetUp = true;
    previewRecoveryStatus.isCreatedByCurrentUser = true;
    previewRecoveryStatus.createdByName = "Alper";
    allPreviewVault.filter((vault) => vault.role === "owner" && !vault.recoveryOptOut).forEach((vault) => (vault.recoverable = true));
    return resolveAfterDelay([] as string[]);
  },
  enableRecoveryForAllVault: () => {
    allPreviewVault.filter((vault) => vault.role === "owner" && !vault.recoveryOptOut).forEach((vault) => (vault.recoverable = true));
    return resolveAfterDelay([] as string[]);
  },
  enableVaultRecovery: (vaultID: string) => resolveAfterDelay(setPreviewVaultRecovery(vaultID, true)),
  disableVaultRecovery: (vaultID: string) => resolveAfterDelay(setPreviewVaultRecovery(vaultID, false)),
  startAccountRecovery: (accountID: string) => resolveAfterDelay(`https://tryhisar.com/recover/r7Kp2xQ9mVt4Lw8Nc3Bz6Hd-${accountID}`),
  listAllPendingRecovery: () => resolveAfterDelay(structuredClone(allPreviewPendingRecovery)),
  completeAccountRecovery: (accountID: string) => {
    const pending = allPreviewPendingRecovery.find((candidate) => candidate.accountId === accountID);
    allPreviewPendingRecovery = allPreviewPendingRecovery.filter((candidate) => candidate.accountId !== accountID);
    const firstName = pending?.name.split(" ")[0] ?? accountID;
    return new Promise<string[]>((resolve) => setTimeout(() => resolve(pending?.unrecoverableVaultCount ? [`${firstName}'s personal vault`] : []), 600));
  },
  recoverAccount: (recoveryLink: string, email: string, newPassword: string): Promise<RecoverAccountResult> => {
    if (!recoveryLink.includes("/recover/")) {
      return Promise.reject(new Error("paste the full recovery link you received from your admin"));
    }
    if (newPassword.length < 10) {
      return Promise.reject(new Error("use a master password with at least 10 characters"));
    }
    previewSignedOutEmail = email.trim().toLowerCase();
    return new Promise((resolve) => setTimeout(() => resolve({ status: previewStatus(), secretKey: "H1-GVC2KA-R8MPL-4XQZT-7HN2C-WK5DB-Q3J6RA" }), 600));
  },
  openURL: (link: string) => {
    window.open(link, "_blank", "noopener");
    return Promise.resolve();
  },
};
