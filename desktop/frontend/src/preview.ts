// Development-only stand-in for the Go backend, used with `npm run preview:ui` to view the UI in a browser.
import type { ItemDetail, PasswordConfig, SignUpResult, Status, VaultSummary } from "./api";

const allPreviewVault: VaultSummary[] = [
  { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0 },
  { id: "family", name: "Family", type: "shared", role: "owner", itemCount: 0 },
  { id: "work", name: "Work", type: "shared", role: "reader", itemCount: 0 },
];

function createPreviewItem(id: string, title: string, username: string, link: string, password: string, extra: Partial<ItemDetail> = {}): ItemDetail {
  return { id, revision: "rev", title, username, urls: link ? [link] : [], password, notes: "", fields: [], ...extra };
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
  ],
  family: [
    createPreviewItem("netflix", "Netflix", "family@example.com", "https://netflix.com", "netflix-family-pass", { notes: "Shared family account" }),
    createPreviewItem("wifi", "Home Wi-Fi", "", "", "wifi-password-123", { fields: [{ name: "Network", value: "Hisar-Home", concealed: false }] }),
    createPreviewItem("spotify", "Spotify Family", "family@example.com", "https://spotify.com", "Tg5!kP9#vL2@"),
  ],
  work: [createPreviewItem("vpn", "Office VPN", "alper", "https://vpn.example.com", "Vq8@Lm3!Zr6#")],
};

function getInitialState(): string {
  return new URLSearchParams(window.location.search).get("state") ?? "unlocked";
}

let previewState = getInitialState();
let isPreviewBiometricEnabled = new URLSearchParams(window.location.search).get("touchId") === "1";
const previewStatus = (): Status => ({
  state: previewState,
  server: "https://tryhisar.com",
  email: "alper@tryhisar.com",
  name: "Alper",
  biometricAvailable: true,
  biometricEnabled: isPreviewBiometricEnabled,
  biometricReady: isPreviewBiometricEnabled && previewState === "locked",
});
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
    const vault = { id: name.toLowerCase(), name, type: "shared", role: "owner", itemCount: 0 };
    allPreviewVault.push(vault);
    allPreviewItemByVaultID[vault.id] = [];
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
  openURL: (link: string) => {
    window.open(link, "_blank", "noopener");
    return Promise.resolve();
  },
};
