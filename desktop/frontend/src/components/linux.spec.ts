import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createApprovalEvent, createForwardedApprovalEvent, createStatus } from "../testing";
import { approvalStore, failApproval } from "../approval";
import SSHApprovalWindow from "./SSHApprovalWindow.vue";
import SecuritySection from "./SecuritySection.vue";
import UnlockView from "./UnlockView.vue";

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
