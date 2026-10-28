import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createStatus, createUpdateStatus } from "../testing";

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
const UpdatesSection = (await import("./UpdatesSection.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { status: createStatus("unlocked"), errorMessage: "", noticeMessage: "", isBusy: false, updateStatus: null });
  api.getUpdateStatus.mockResolvedValue(createUpdateStatus());
});

describe("UpdatesSection", () => {
  it("shows the installed version and where updates come from", async () => {
    store.status = { ...createStatus("unlocked"), server: "https://hisar.example.com" };
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    expect(api.getUpdateStatus).toHaveBeenCalled();
    expect(wrapper.find('[data-testid="installed-version"]').text()).toBe("Hisar 0.2.1");
    expect(wrapper.find('[data-testid="update-explanation"]').text()).toContain("https://hisar.example.com");
    expect(wrapper.find('[data-testid="check-for-updates"]').attributes("disabled")).toBeUndefined();
    expect((wrapper.find('input[name="automaticUpdateCheck"]').element as HTMLInputElement).checked).toBe(true);
  });

  it("checks for updates when asked", async () => {
    api.checkForUpdates.mockResolvedValue(createUpdateStatus());
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    await wrapper.find('[data-testid="check-for-updates"]').trigger("click");
    await flushPromises();
    expect(api.checkForUpdates).toHaveBeenCalledTimes(1);
    expect(store.noticeMessage).toBe("Checking for updates…");
  });

  it("shows why a check failed", async () => {
    api.checkForUpdates.mockRejectedValue(new Error("the updater has not started yet"));
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    await wrapper.find('[data-testid="check-for-updates"]').trigger("click");
    await flushPromises();
    expect(store.errorMessage).toBe("the updater has not started yet");
  });

  it("turns automatic checks off and on", async () => {
    api.setAutomaticUpdateCheck.mockResolvedValue(createUpdateStatus({ isAutomaticCheckEnabled: false }));
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    await wrapper.find('input[name="automaticUpdateCheck"]').setValue(false);
    await flushPromises();
    expect(api.setAutomaticUpdateCheck).toHaveBeenCalledWith(false);
    expect(store.updateStatus?.isAutomaticCheckEnabled).toBe(false);
    expect((wrapper.find('input[name="automaticUpdateCheck"]').element as HTMLInputElement).checked).toBe(false);

    api.setAutomaticUpdateCheck.mockResolvedValue(createUpdateStatus({ isAutomaticCheckEnabled: true }));
    await wrapper.find('input[name="automaticUpdateCheck"]').setValue(true);
    await flushPromises();
    expect(api.setAutomaticUpdateCheck).toHaveBeenLastCalledWith(true);
    expect(store.updateStatus?.isAutomaticCheckEnabled).toBe(true);
  });

  it("disables checking in a development build", async () => {
    api.getUpdateStatus.mockResolvedValue(createUpdateStatus({ version: "development", buildNumber: "", isAvailable: false }));
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="installed-version"]').text()).toBe("Hisar development build");
    expect(wrapper.find('[data-testid="update-explanation"]').text()).toContain("cannot update itself");
    expect(wrapper.find('[data-testid="check-for-updates"]').attributes("disabled")).toBeDefined();
    expect(wrapper.find('input[name="automaticUpdateCheck"]').attributes("disabled")).toBeDefined();
  });

  it("asks to sign in before checking", async () => {
    api.getUpdateStatus.mockResolvedValue(createUpdateStatus({ isSignedIn: false }));
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="update-explanation"]').text()).toContain("Sign in to check for updates");
    expect(wrapper.find('[data-testid="check-for-updates"]').attributes("disabled")).toBeDefined();
  });
});

describe("UpdatesSection on Linux and Windows", () => {
  it("finds a newer version and opens its download page", async () => {
    api.getUpdateStatus.mockResolvedValue(createUpdateStatus({ isNotifyOnly: true, buildNumber: "" }));
    const available = createUpdateStatus({
      isNotifyOnly: true,
      hasChecked: true,
      availableVersion: "0.3.0",
      downloadURL: "https://github.com/tosunlab/hisar/releases/tag/desktop-0.3.0",
    });
    api.checkForUpdates.mockResolvedValue(available);
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="update-explanation"]').text()).toContain("you download and install them");
    await wrapper.find('[data-testid="check-for-updates"]').trigger("click");
    await flushPromises();
    expect(store.noticeMessage).toBe("Hisar 0.3.0 is available");
    expect(wrapper.find('[data-testid="update-explanation"]').text()).toContain("Hisar 0.3.0 is available");
    await wrapper.find('[data-testid="download-update"]').trigger("click");
    expect(api.openURL).toHaveBeenCalledWith("https://github.com/tosunlab/hisar/releases/tag/desktop-0.3.0");
  });

  it("says when this version is the newest", async () => {
    api.getUpdateStatus.mockResolvedValue(createUpdateStatus({ isNotifyOnly: true }));
    api.checkForUpdates.mockResolvedValue(createUpdateStatus({ isNotifyOnly: true, hasChecked: true }));
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    await wrapper.find('[data-testid="check-for-updates"]').trigger("click");
    await flushPromises();
    expect(store.noticeMessage).toBe("Hisar is up to date");
    expect(wrapper.find('[data-testid="update-explanation"]').text()).toBe("Hisar is up to date.");
    expect(wrapper.find('[data-testid="download-update"]').exists()).toBe(false);
  });

  it("installs a newer version in a copy from a Linux package", async () => {
    const available = createUpdateStatus({ isNotifyOnly: true, hasChecked: true, availableVersion: "0.3.0", installKind: "deb" });
    api.getUpdateStatus.mockResolvedValue(available);
    api.installUpdate.mockResolvedValue(available);
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="update-explanation"]').text()).toContain("opens it in your software installer");
    expect(wrapper.find('[data-testid="download-update"]').exists()).toBe(false);
    await wrapper.find('[data-testid="install-update"]').trigger("click");
    await flushPromises();
    expect(api.installUpdate).toHaveBeenCalled();
    expect(api.openURL).not.toHaveBeenCalled();
    expect(store.noticeMessage).toBe("Hisar 0.3.0 is verified. Install it in your software installer, then open Hisar again.");
  });

  it("replaces an AppImage and relaunches it", async () => {
    api.getUpdateStatus.mockResolvedValue(createUpdateStatus({ isNotifyOnly: true, hasChecked: true, availableVersion: "0.3.0", installKind: "appimage" }));
    api.installUpdate.mockRejectedValue(new Error("the downloaded update is not signed by Hisar; it was not installed"));
    const wrapper = mount(UpdatesSection);
    await flushPromises();
    const installButton = wrapper.find('[data-testid="install-update"]');
    expect(installButton.text()).toBe("Install and Relaunch");
    await installButton.trigger("click");
    await flushPromises();
    expect(store.errorMessage).toContain("not signed by Hisar");
  });
});
