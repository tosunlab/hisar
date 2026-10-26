import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createForwardedApprovalEvent, createLoginItemStatus, createStatus } from "../testing";
import { approvalStore } from "../approval";
import SignInView from "./SignInView.vue";
import UpdateRequiredView from "./UpdateRequiredView.vue";
import SSHApprovalWindow from "./SSHApprovalWindow.vue";
import AppearanceSection from "./AppearanceSection.vue";

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
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const OpenAtLoginSection = (await import("./OpenAtLoginSection.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  Object.defineProperty(navigator, "platform", { value: "Linux x86_64", configurable: true });
  store.status = { ...createStatus("unlocked"), biometricAvailable: false };
  api.getLoginItemStatus.mockResolvedValue(createLoginItemStatus());
});

afterEach(() => {
  Object.defineProperty(navigator, "platform", { value: "MacIntel", configurable: true });
});

describe("Settings on Linux", () => {
  it("says this computer unlocks with the master password and never mentions a Mac", async () => {
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    expect(wrapper.text()).toContain("Unlocking with a fingerprint is not available on this computer yet");
    expect(wrapper.text()).toContain("Hisar locks once this computer has had no keyboard or mouse input");
    expect(wrapper.text()).not.toContain("Mac");
  });

  it("keeps Hisar in the system tray", async () => {
    const wrapper = mount(OpenAtLoginSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="login-item-state"]').text()).toContain("stays in the system tray");
  });
});

describe("Authentication on Linux", () => {
  it("names this computer when creating an account", async () => {
    const wrapper = mount(SignInView);
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    expect(wrapper.get('[data-testid="create-account-note"]').text()).toContain("Your master password never leaves this computer.");
    wrapper.unmount();
  });

  it("names this computer when an update is required", () => {
    store.status.updateRequired = { currentVersion: "0.1.0", minimumVersion: "0.2.0", updateURL: "" };
    const wrapper = mount(UpdateRequiredView);
    expect(wrapper.get('[data-testid="update-required-versions"]').text()).toBe("This server needs Hisar 0.2.0 or later. This computer has 0.1.0.");
    expect(wrapper.text()).toContain("items stay encrypted on this computer and on the server.");
    expect(wrapper.text()).not.toContain("Mac");
    wrapper.unmount();
  });

  it("distinguishes forwarded requests from this computer", async () => {
    approvalStore.allApproval = [createForwardedApprovalEvent()];
    api.listAllPendingSSHApproval.mockResolvedValue([]);
    const wrapper = mount(SSHApprovalWindow);
    await flushPromises();
    expect(wrapper.get('[data-testid="approval-forwarded"]').text()).toContain("not from this computer");
    wrapper.unmount();
    approvalStore.allApproval = [];
  });

  it("follows the system appearance without naming macOS", async () => {
    const wrapper = mount(AppearanceSection);
    await flushPromises();
    expect(wrapper.text()).toContain("System switches between light and dark with the system.");
    expect(wrapper.text()).not.toContain("macOS");
    wrapper.unmount();
  });
});
