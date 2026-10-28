import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createForwardedApprovalEvent, createStatus } from "../testing";
import { approvalStore } from "../approval";
import SSHApprovalWindow from "./SSHApprovalWindow.vue";

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
