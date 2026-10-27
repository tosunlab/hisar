import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createStatus } from "../testing";
import type { SignUpResult } from "../api";

const mock = vi.hoisted(() => ({ api: {} as ReturnType<typeof createApiMock> }));
vi.mock("../api", async () => {
  const { createApiMock } = await import("../testing");
  mock.api = createApiMock();
  return { api: mock.api, getErrorMessage: (error: unknown) => error instanceof Error ? error.message : String(error) };
});
const model = await import("../store");
const PendingRecoveryView = (await import("./PendingRecoveryView.vue")).default;
const entry = { operationId: "pending-1", kind: "signup", origin: "https://hisar.test", accountId: "account", email: "alice@example.com", name: "Alice", phase: "needs-finishing", kitSaved: false, manualUnlock: true, keychainProtected: false, revision: 1, problems: ["The keychain could not save this Secret Key."] };
enableAutoUnmount(afterEach);

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(model.store, { status: createStatus("signed-out"), pendingRecovery: [{ ...entry }], isRecoveryOpen: true, errorMessage: "", isBusy: false });
  mock.api.listPendingRecovery.mockResolvedValue([{ ...entry }]);
});

describe("Pending recovery", () => {
  it("shows partial completion without claiming success or exposing factors", () => {
    const wrapper = mount(PendingRecoveryView);
    expect(wrapper.text()).toContain("Your account was created, but setup did not finish on this device.");
    expect(wrapper.text()).toContain("This device does not persist the new Secret Key");
    expect(wrapper.text()).not.toContain("The operation completed");
    expect(mock.api.resumePendingRecovery).not.toHaveBeenCalled();
  });

  it("does not send an uncommitted change before saved-kit acknowledgement", async () => {
    model.store.pendingRecovery = [{ ...entry, phase: "awaiting-kit" }];
    mock.api.savePendingRecoveryEmergencyKit.mockResolvedValue("/tmp/Hisar Emergency Kit.pdf");
    const wrapper = mount(PendingRecoveryView);
    const password = wrapper.find('input[name="pendingRecoveryPassword"]');
    await password.setValue("password");
    expect(wrapper.findAll("button").find((button) => button.text() === "Continue")!.attributes("disabled")).toBeDefined();
    await wrapper.findAll("button").find((button) => button.text() === "Save Emergency Kit…")!.trigger("click");
    await flushPromises();
    await wrapper.find('input[name="pendingRecoveryKitSaved"]').setValue(true);
    mock.api.acknowledgePendingRecovery.mockResolvedValue(undefined);
    mock.api.listPendingRecovery.mockResolvedValue([{ ...entry, phase: "prepared", kitSaved: true }]);
    await wrapper.findAll("button").find((button) => button.text() === "Confirm saved")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("Your Emergency Kit is confirmed saved. Continue to send this change.");
    expect(mock.api.resumePendingRecovery).not.toHaveBeenCalled();
    expect(model.store.status.state).toBe("signed-out");
  });

  it("requires an actual saved kit and explicit confirmation before acknowledgement", async () => {
    model.store.pendingRecovery = [{ ...entry, phase: "delivery-pending" }];
    mock.api.savePendingRecoveryEmergencyKit.mockResolvedValue("");
    const wrapper = mount(PendingRecoveryView);
    await wrapper.find('input[name="pendingRecoveryPassword"]').setValue("password");
    const save = wrapper.findAll("button").find((button) => button.text() === "Save Emergency Kit…")!;
    await save.trigger("click");
    await flushPromises();
    expect(wrapper.find('input[name="pendingRecoveryKitSaved"]').exists()).toBe(false);
    expect(mock.api.acknowledgePendingRecovery).not.toHaveBeenCalled();
    mock.api.savePendingRecoveryEmergencyKit.mockResolvedValue("/tmp/Hisar Emergency Kit.pdf");
    await save.trigger("click");
    await flushPromises();
    await wrapper.find('input[name="pendingRecoveryKitSaved"]').setValue(true);
    const confirm = wrapper.findAll("button").find((button) => button.text() === "Confirm saved")!;
    expect(confirm.attributes("disabled")).toBeUndefined();
    mock.api.acknowledgePendingRecovery.mockResolvedValue(undefined);
    mock.api.listPendingRecovery.mockResolvedValue([]);
    await confirm.trigger("click");
    await flushPromises();
    expect(mock.api.acknowledgePendingRecovery).toHaveBeenCalledWith("pending-1", "password");
    expect(model.store.pendingRecovery).toEqual([]);
    expect((wrapper.get('input[name="pendingRecoveryPassword"]').element as HTMLInputElement).value).toBe("");
  });

  it("keeps Secret Keys after failure and clears only a rejected master password", async () => {
    const wrapper = mount(PendingRecoveryView);
    const password = wrapper.get('input[name="pendingRecoveryPassword"]');
    const key = wrapper.get('input[name="pendingRecoverySecretKey"]');
    const prior = wrapper.get('input[name="pendingRecoveryPriorSecretKey"]');
    await password.setValue("password");
    await key.setValue("H1-SAVED");
    await prior.setValue("H1-PRIOR");
    mock.api.resumePendingRecovery.mockRejectedValue(new Error("Network unavailable"));
    await wrapper.findAll("button").find(button => button.text() === "Continue")!.trigger("click");
    await flushPromises();
    expect((password.element as HTMLInputElement).value).toBe("password");
    mock.api.resumePendingRecovery.mockRejectedValue(new Error("pending-recovery-password: The master password could not open this setup"));
    await wrapper.findAll("button").find(button => button.text() === "Continue")!.trigger("click");
    await flushPromises();
    expect((password.element as HTMLInputElement).value).toBe("");
    expect((key.element as HTMLInputElement).value).toBe("H1-SAVED");
    expect((prior.element as HTMLInputElement).value).toBe("H1-PRIOR");
    expect(wrapper.text()).not.toContain("pending-recovery-password:");
    model.store.status = { ...model.store.status, server: "https://other.example" };
    await flushPromises();
    expect((key.element as HTMLInputElement).value).toBe("");
    expect((prior.element as HTMLInputElement).value).toBe("");
  });

  it("does not unlock when recovery completes after a lock", async () => {
    const result = Promise.withResolvers<SignUpResult>();
    mock.api.resumePendingRecovery.mockReturnValue(result.promise);
    const recovery = model.resumePendingRecovery("pending-1", "password");
    mock.api.getStatus.mockResolvedValue(createStatus("locked"));
    await model.handleLocked();
    result.resolve({ status: createStatus("unlocked"), secretKey: "", pendingOperationId: "pending-1", setupState: "delivery-pending", problems: [] });
    expect(await recovery).toBe(false);
    expect(model.store.status.state).toBe("locked");
    expect(model.store.pendingSecretKey).toBe("");
  });
});
