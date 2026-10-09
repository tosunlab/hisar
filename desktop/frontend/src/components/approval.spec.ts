import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createApprovalEvent } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const eventListener = vi.hoisted(() => ({ allByName: {} as Record<string, (event: { data: unknown }) => void> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return { api: apiMock.current, getErrorMessage: (error: unknown) => String(error) };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      eventListener.allByName[name] = listener;
      return () => delete eventListener.allByName[name];
    },
  },
}));

const { approvalStore } = await import("../approval");
const SSHApprovalWindow = (await import("./SSHApprovalWindow.vue")).default;
const api = apiMock.current;

function sendEvent(name: string, data: unknown): void {
  eventListener.allByName[name]({ data });
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(approvalStore, { allApproval: [], failureMessage: "", isAuthorizing: false, isForAllApplication: false });
  api.listAllPendingSSHApproval.mockResolvedValue([]);
});

describe("SSHApprovalWindow", () => {
  it("shows the requesting app, the key and its details", async () => {
    api.listAllPendingSSHApproval.mockResolvedValue([createApprovalEvent({ appIconURL: "data:image/png;base64,AA==" })]);
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("Hisar Access Requested");
    expect(wrapper.find('[data-testid="approval-app"]').text()).toBe("Terminal");
    expect(wrapper.find('[data-testid="app-icon"]').attributes("src")).toBe("data:image/png;base64,AA==");
    expect(wrapper.find('[data-testid="approval-key"]').text()).toContain("Laptop key");
    expect(wrapper.find('[data-testid="approval-details"]').exists()).toBe(false);
    await wrapper.find('[data-testid="approval-key"]').trigger("click");
    const details = wrapper.find('[data-testid="approval-details"]').text();
    expect(details).toContain("github.com");
    expect(details).toContain("/usr/bin/ssh");
    expect(details).toContain("SHA256:hisarTestFingerprint");
    wrapper.unmount();
  });

  it("authorizes with Touch ID, for all applications when checked", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    await wrapper.find('input[name="isForAllApplication"]').setValue(true);
    await wrapper.find('[data-testid="authorize-approval"]').trigger("click");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", true, true);
    expect(wrapper.find('[data-testid="authorize-approval"]').text()).toBe("Waiting for Touch ID…");
    expect(wrapper.find('[data-testid="deny-approval"]').attributes("disabled")).toBeDefined();

    sendEvent("ssh-approval-closed", "request-1");
    await flushPromises();
    expect(wrapper.find('[data-testid="ssh-approval"]').exists()).toBe(false);
    expect(approvalStore.isForAllApplication).toBe(false);
    wrapper.unmount();
  });

  it("lets the user try again after Touch ID fails", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    await wrapper.find('[data-testid="authorize-approval"]').trigger("click");
    sendEvent("ssh-approval-failed", { requestID: "request-1", message: "Touch ID did not confirm it's you. Try again, or deny." });
    await flushPromises();
    expect(wrapper.find('[role="alert"]').text()).toContain("Touch ID did not confirm");
    expect(wrapper.find('[data-testid="authorize-approval"]').text()).toBe("Authorize");
    await wrapper.find('[data-testid="authorize-approval"]').trigger("click");
    expect(api.respondSSHApproval).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });

  it("denies with the button or Escape and moves to the next request", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    sendEvent("ssh-approval", createApprovalEvent({ requestID: "request-2", appName: "Visual Studio Code", isForwarded: true, isTouchIDApproval: false }));
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    expect(wrapper.text()).toContain("1 more waiting");
    await wrapper.find('[data-testid="deny-approval"]').trigger("click");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", false, false);
    expect(wrapper.find('[data-testid="approval-app"]').text()).toBe("Visual Studio Code");
    expect(wrapper.find('[data-testid="approval-forwarded"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="authorize-approval"]').text()).toBe("Authorize");

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-2", false, false);
    expect(wrapper.find('[data-testid="ssh-approval"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("authorizes with Enter and ignores keys while waiting", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await flushPromises();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledTimes(1);
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", true, false);
    wrapper.unmount();
  });
});
