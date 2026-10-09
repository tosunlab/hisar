import { vi } from "vitest";
import type { ApprovalEvent, ItemDetail, SSHAgentStatus, Status, VaultSummary } from "./api";

export function createStatus(state: string): Status {
  return { state, server: "https://hisar.test", email: "alice@example.com", name: "Alice", biometricAvailable: false, biometricEnabled: false, biometricReady: false };
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
    clientName: "ssh",
    clientPath: "/usr/bin/ssh",
    keyTitle: "Laptop key",
    fingerprint: "SHA256:hisarTestFingerprint",
    destinationHost: "github.com",
    destinationFingerprint: "SHA256:hostFingerprint",
    isForwarded: false,
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
    generatePassword: vi.fn(),
    generateSSHKey: vi.fn(),
    importSSHKey: vi.fn(),
    getSSHAgentStatus: vi.fn().mockResolvedValue(createSSHAgentStatus()),
    configureSSHConfig: vi.fn(),
    removeSSHConfig: vi.fn(),
    respondSSHApproval: vi.fn().mockResolvedValue(undefined),
    copyField: vi.fn(),
    copyText: vi.fn(),
    openURL: vi.fn().mockResolvedValue(undefined),
    unlockWithBiometric: vi.fn(),
    enableBiometricUnlock: vi.fn(),
    disableBiometricUnlock: vi.fn(),
    setTouchIDPromptFrame: vi.fn().mockResolvedValue(undefined),
    clearTouchIDPromptFrame: vi.fn().mockResolvedValue(undefined),
    cancelTouchIDPrompt: vi.fn().mockResolvedValue(undefined),
    hasNativeTouchIDView: false,
    saveNewAccountEmergencyKit: vi.fn(),
    saveEmergencyKit: vi.fn(),
    forgetNewAccountSecretKey: vi.fn().mockResolvedValue(undefined),
  };
}
