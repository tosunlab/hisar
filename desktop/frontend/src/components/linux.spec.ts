import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createApprovalEvent, createEmergencyKit, createForwardedApprovalEvent, createStatus } from "../testing";
import { approvalStore, failApproval } from "../approval";
import SSHApprovalWindow from "./SSHApprovalWindow.vue";
import SecuritySection from "./SecuritySection.vue";
import UnlockView from "./UnlockView.vue";
import SecretKeyView from "./SecretKeyView.vue";

vi.mock("@wailsio/runtime", () => ({
  Events: { On: () => () => undefined },
}));

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] }),
  };
});

const { store } = await import("../store");
const api = apiMock.current;

beforeEach(() => {
  Object.defineProperty(navigator, "platform", { value: "Linux x86_64", configurable: true });
  store.status = { ...createStatus("unlocked"), biometricAvailable: false };
  Object.assign(approvalStore, { allApproval: [], failureMessage: "", isAuthorizing: false, isPasswordChosen: false, isExpanded: false });
});

afterEach(() => {
  Object.defineProperty(navigator, "platform", { value: "MacIntel", configurable: true });
});

describe("Authentication on Linux", () => {
  it("distinguishes forwarded requests from this computer", async () => {
    approvalStore.allApproval = [createForwardedApprovalEvent()];
    api.listAllPendingSSHApproval.mockResolvedValue([]);
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    expect(wrapper.get('[data-testid="approval-forwarded"]').text()).toContain("not from this computer");
    wrapper.unmount();
    approvalStore.allApproval = [];
  });
});

describe("Fingerprint unlock copy on Linux", () => {
  it("names fingerprint in the Security row and shows the backend's unavailable reason", () => {
    store.status = { ...createStatus("unlocked"), biometricAvailable: false, biometricUnavailableReason: "No fingerprint is enrolled on this computer." };
    const wrapper = mount(SecuritySection);
    expect(wrapper.get('[data-testid="security-section"] strong').text()).toBe("Unlock with fingerprint");
    expect(wrapper.get('[data-testid="security-section"]').text()).toContain("No fingerprint is enrolled on this computer.");
    wrapper.unmount();
  });

  it("labels the unlock button for a fingerprint", async () => {
    store.status = { ...createStatus("locked"), biometricAvailable: true, biometricEnabled: true, biometricReady: true };
    api.unlockWithBiometric.mockReturnValue(new Promise(() => undefined));
    const wrapper = mount(UnlockView);
    await flushPromises();
    const button = wrapper.get('[data-testid="use-touch-id"]');
    expect(button.attributes("aria-label")).toBe("Unlock with fingerprint");
    expect(button.attributes("title")).toBe("Unlock with fingerprint");
    wrapper.unmount();
  });

  it("asks to scan a finger in the SSH approval window", async () => {
    approvalStore.allApproval = [createApprovalEvent()];
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    expect(wrapper.get('[data-testid="approval-touch-id"]').text()).toContain("Scan your finger to approve");
    wrapper.unmount();
  });

  it("offers Try fingerprint again after a failed scan", async () => {
    approvalStore.allApproval = [createApprovalEvent({ isCompact: true })];
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    failApproval("request-1", "Fingerprint did not confirm it's you. Try again, or deny.");
    await flushPromises();
    expect(wrapper.get('[data-testid="authorize-approval"]').text()).toBe("Try fingerprint again");
    wrapper.unmount();
  });

  it("keeps the request open when a chosen password cancels a Fingerprint check", () => {
    approvalStore.allApproval = [createApprovalEvent()];
    approvalStore.isPasswordChosen = true;
    approvalStore.failureMessage = "";
    failApproval("request-1", "Fingerprint check was cancelled");
    expect(approvalStore.failureMessage).toBe("");
  });
});

describe("Fingerprint silent waits on Linux", () => {
  it("scans for a finger only while the window is focused", async () => {
    const hasFocus = vi.spyOn(document, "hasFocus").mockReturnValue(false);
    store.status = { ...createStatus("locked"), biometricAvailable: true, biometricEnabled: true, biometricReady: true };
    api.unlockWithBiometric.mockReturnValue(new Promise(() => undefined));
    const wrapper = mount(UnlockView);
    await flushPromises();
    expect(api.unlockWithBiometric).not.toHaveBeenCalled();
    window.dispatchEvent(new Event("focus"));
    await flushPromises();
    expect(api.unlockWithBiometric).toHaveBeenCalledTimes(1);
    window.dispatchEvent(new Event("blur"));
    await flushPromises();
    expect(api.cancelTouchIDPrompt).toHaveBeenCalled();
    wrapper.unmount();
    hasFocus.mockRestore();
  });

  it("explains fingerprint setup is pending without the Mac wording", () => {
    store.status = { ...createStatus("unlocked"), biometricAvailable: true, biometricSetupError: "fprintd: enrollment failed" };
    const wrapper = mount(SecuritySection);
    const text = wrapper.get('[data-testid="security-section"]').text();
    expect(text).toContain("Fingerprint unlock needs your master password to set up again. Your master password still unlocks Hisar.");
    expect(text).toContain("fprintd: enrollment failed");
    wrapper.unmount();
  });

  it("shows the scan prompt and cancels the Emergency Kit fingerprint", async () => {
    store.status = { ...createStatus("unlocked"), biometricEnabled: true, emergencyKit: createEmergencyKit({ isAuthorized: false }) };
    api.authorizeNewAccountEmergencyKitWithBiometric.mockReturnValue(new Promise(() => undefined));
    const wrapper = mount(SecretKeyView);
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text() === "Continue with fingerprint")!.trigger("click");
    await flushPromises();
    expect(wrapper.get('[data-testid="fingerprint-waiting"]').text()).toBe("Scan your finger…");
    await wrapper.get('[data-testid="cancel-fingerprint"]').trigger("click");
    expect(api.cancelTouchIDPrompt).toHaveBeenCalled();
    wrapper.unmount();
  });
});
