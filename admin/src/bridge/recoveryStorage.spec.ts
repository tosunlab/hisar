import { IDBFactory } from "fake-indexeddb";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listRecovery, loadRecovery, removeRecovery, stageRecovery } from "./recoveryStorage";
import type { RecoveryMetadata } from "./recoveryStorage";

const metadata: RecoveryMetadata = { operationId: "operation", kind: "signup", origin: "https://hisar.test", accountId: "account", email: "ada@example.com", name: "Ada", phase: "awaiting-kit", revision: 1 };
const envelope = "opaque-sanitized-encrypted-operation";

beforeEach(() => { vi.stubGlobal("indexedDB", new IDBFactory()); });
afterEach(() => { vi.unstubAllGlobals(); });

describe("Durable browser recovery", () => {
  it("creates an operation once and rejects a stale update after remote commit starts", async () => {
    await stageRecovery(metadata, envelope);
    await expect(stageRecovery(metadata, envelope)).rejects.toThrow("changed in another page");
    await stageRecovery({ ...metadata, revision: 2, phase: "outcome-unknown" }, envelope);
    await expect(stageRecovery({ ...metadata, revision: 2, phase: "prepared" }, envelope)).rejects.toThrow("changed in another page");
    expect((await loadRecovery(metadata.operationId))?.metadata.phase).toBe("outcome-unknown");
  });

  it("permits only one competing update at the same revision", async () => {
    await stageRecovery(metadata, envelope);
    const outcomes = await Promise.allSettled([
      stageRecovery({ ...metadata, revision: 2, phase: "prepared" }, envelope),
      stageRecovery({ ...metadata, revision: 2, phase: "outcome-unknown" }, envelope),
    ]);
    expect(outcomes.filter((outcome) => outcome.status === "fulfilled")).toHaveLength(1);
    expect((await loadRecovery(metadata.operationId))?.metadata.revision).toBe(2);
  });

  it("does not let a stale discard erase an operation whose remote outcome is unknown", async () => {
    await stageRecovery(metadata, envelope);
    await stageRecovery({ ...metadata, revision: 2, phase: "outcome-unknown" }, envelope);
    await expect(removeRecovery(metadata.operationId, 1)).rejects.toThrow("changed in another page");
    expect(await listRecovery()).toHaveLength(1);
    await removeRecovery(metadata.operationId, 2);
    expect(await loadRecovery(metadata.operationId)).toBeNull();
    await expect(stageRecovery({ ...metadata, revision: 3 }, envelope)).rejects.toThrow("removed or replaced");
  });

  it("keeps the operation bound to its account and origin", async () => {
    await stageRecovery(metadata, envelope);
    await expect(stageRecovery({ ...metadata, revision: 2, accountId: "other" }, envelope)).rejects.toThrow("changed in another page");
    await expect(stageRecovery({ ...metadata, revision: 2, origin: "https://other.test" }, envelope)).rejects.toThrow("changed in another page");
    expect((await loadRecovery(metadata.operationId))?.metadata.revision).toBe(1);
  });

  it("bounds record sizes and the number of operations", async () => {
    await expect(stageRecovery(metadata, "x".repeat(1024 * 1024 + 1))).rejects.toThrow("storage limit");
    await expect(stageRecovery({ ...metadata, name: "x".repeat(64 * 1024) }, envelope)).rejects.toThrow("storage limit");
    for (let index = 0; index < 100; index++) await stageRecovery({ ...metadata, operationId: `operation-${index}` }, envelope);
    await expect(stageRecovery(metadata, envelope)).rejects.toThrow("100 recovery operations");
    expect(await listRecovery()).toHaveLength(100);
  });
});
