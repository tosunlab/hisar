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

  it("starts Touch ID in the window as soon as a request arrives", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    expect(api.setApprovalTouchIDFrame).toHaveBeenCalled();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", true, false);
    expect(wrapper.find('[data-testid="approval-touch-id"]').text()).toContain("Touch ID to approve");
    expect(wrapper.find('[data-testid="authorize-approval"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="deny-approval"]').attributes("disabled")).toBeUndefined();

    await wrapper.find('input[name="isForAllApplication"]').setValue(true);
    expect(api.setSSHApprovalForAllApplication).toHaveBeenCalledWith("request-1", true);

    sendEvent("ssh-approval-closed", "request-1");
    await flushPromises();
    expect(wrapper.find('[data-testid="ssh-approval"]').exists()).toBe(false);
    expect(api.clearApprovalTouchIDFrame).toHaveBeenCalled();
    expect(approvalStore.isForAllApplication).toBe(false);
    wrapper.unmount();
  });

  it("offers Authorize again after Touch ID fails", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    await flushPromises();
    sendEvent("ssh-approval-failed", { requestID: "request-1", message: "Touch ID did not confirm it's you. Try again, or deny." });
    await flushPromises();
    expect(wrapper.find('[role="alert"]').text()).toContain("Touch ID did not confirm");
    expect(wrapper.find('[data-testid="approval-touch-id"]').exists()).toBe(false);
    await wrapper.find('[data-testid="authorize-approval"]').trigger("click");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledTimes(2);
    expect(wrapper.find('[data-testid="approval-touch-id"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("denies while Touch ID waits, with the button or Escape", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent());
    sendEvent("ssh-approval", createApprovalEvent({ requestID: "request-2", appName: "Visual Studio Code", isForwarded: true }));
    await flushPromises();
    await wrapper.find('[data-testid="deny-approval"]').trigger("click");
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", false, false);
    expect(wrapper.find('[data-testid="approval-app"]').text()).toBe("Visual Studio Code");
    expect(wrapper.find('[data-testid="approval-forwarded"]').exists()).toBe(true);
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-2", true, false);

    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-2", false, false);
    expect(wrapper.find('[data-testid="ssh-approval"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("waits for a click on Macs without Touch ID", async () => {
    const wrapper = mount(SSHApprovalWindow);
    sendEvent("ssh-approval", createApprovalEvent({ isTouchIDApproval: false }));
    sendEvent("ssh-approval", createApprovalEvent({ requestID: "request-2", isTouchIDApproval: false }));
    await flushPromises();
    expect(api.respondSSHApproval).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("1 more waiting");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await flushPromises();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await flushPromises();
    expect(api.respondSSHApproval).toHaveBeenCalledTimes(1);
    expect(api.respondSSHApproval).toHaveBeenCalledWith("request-1", true, false);
    wrapper.unmount();
  });

});
