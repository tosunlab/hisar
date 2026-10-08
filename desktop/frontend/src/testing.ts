import { vi } from "vitest";
import type { ItemDetail, Status, VaultSummary } from "./api";

export function createStatus(state: string): Status {
  return { state, server: "https://hisar.test", email: "alice@example.com", name: "Alice" };
}

export function createItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return { id: "item-1", revision: "rev-1", title: "GitHub", username: "alice", urls: ["https://github.com"], password: "", notes: "", fields: [], ...overrides };
}

export const allTestVault: VaultSummary[] = [
  { id: "personal", name: "Personal", type: "personal", role: "owner" },
  { id: "family", name: "Family", type: "shared", role: "reader" },
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
    copyField: vi.fn(),
    copyText: vi.fn(),
  };
}
