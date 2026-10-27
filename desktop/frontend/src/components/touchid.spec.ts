import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createStatus } from "../testing";
import MainView from "./MainView.vue";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", category: "", title: "", username: "", urls: [], password: "", notes: "", fields: [], publicKey: "", fingerprint: "", keyType: "" }),
  };
});

const storeModule = await import("../store");
const { store } = storeModule;
const UnlockView = (await import("./UnlockView.vue")).default;
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const api = apiMock.current;

function createLockedStatus(biometricReady: boolean) {
  return { ...createStatus("locked"), biometricAvailable: true, biometricEnabled: biometricReady, biometricReady };
}

function createPendingPromise<Value>() {
  let resolvePromise: (value: Value) => void = () => undefined;
  let rejectPromise: (error: Error) => void = () => undefined;
  const promise = new Promise<Value>((resolve, reject) => {
    resolvePromise = resolve;
    rejectPromise = reject;
  });
  return { promise, resolvePromise, rejectPromise };
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { status: createLockedStatus(false), errorMessage: "", isBusy: false, isSettingsOpen: false, allVault: [] });
});

describe("UnlockView with Touch ID", () => {
  it("starts Touch ID right away and places the prompt in the slot", async () => {
    store.status = createLockedStatus(true);
    const pending = createPendingPromise<ReturnType<typeof createStatus>>();
    api.unlockWithBiometric.mockReturnValue(pending.promise);
    const wrapper = mount(UnlockView, { attachTo: document.body });
    const statusRegion = wrapper.get('[data-testid="unlock-status"]').element;
    await flushPromises();
    expect(wrapper.find('[data-testid="touch-id-slot"]').exists()).toBe(true);
    expect(api.setTouchIDPromptFrame).toHaveBeenCalled();
    expect(api.unlockWithBiometric).toHaveBeenCalledTimes(1);
    expect(wrapper.find('input[name="password"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="unlock-status"]').text()).toBe("Touch ID is waiting. Or type your master password.");

    api.getStatus.mockResolvedValue(createStatus("unlocked"));
    api.listAllVault.mockResolvedValue([]);
    pending.resolvePromise({ ...createStatus("unlocked"), biometricAvailable: true, biometricEnabled: true, biometricReady: false });
    await flushPromises();
    expect(store.status.state).toBe("unlocked");
    expect(api.clearTouchIDPromptFrame).toHaveBeenCalled();
    expect(wrapper.get('[data-testid="unlock-status"]').element).toBe(statusRegion);
    expect(wrapper.get('[data-testid="unlock-status"]').text()).toBe("");
    wrapper.unmount();
  });

  it("cancels Touch ID when the master password is submitted", async () => {
    store.status = createLockedStatus(true);
    const pending = createPendingPromise<ReturnType<typeof createStatus>>();
    api.unlockWithBiometric.mockReturnValue(pending.promise);
    api.getStatus.mockResolvedValue(createLockedStatus(true));
    const wrapper = mount(UnlockView, { attachTo: document.body });
    await flushPromises();
    await wrapper.find('input[name="password"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    expect(api.cancelTouchIDPrompt).toHaveBeenCalled();
    expect(api.unlock).toHaveBeenCalledWith("master password");

    pending.rejectPromise(new Error("Touch ID was cancelled"));
    await flushPromises();
    expect(store.errorMessage).toBe("");
    expect(wrapper.find('[data-testid="use-touch-id"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("falls back to the password with the error when Touch ID fails", async () => {
    store.status = createLockedStatus(true);
    api.unlockWithBiometric.mockRejectedValue(new Error("your master password changed; Touch ID unlock was turned off"));
    api.getStatus.mockResolvedValue(createLockedStatus(false));
    const wrapper = mount(UnlockView, { attachTo: document.body });
    await flushPromises();
    expect(store.errorMessage).toContain("turned off");
    expect(wrapper.find('input[name="password"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="use-touch-id"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="unlock-status"]').text()).toBe("");
    wrapper.unmount();
  });

  it("shows only the password field when Touch ID is not ready", async () => {
    const wrapper = mount(UnlockView);
    await flushPromises();
    expect(api.unlockWithBiometric).not.toHaveBeenCalled();
    expect(wrapper.find('input[name="password"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="use-touch-id"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="unlock-status"]').text()).toBe("");
    expect(wrapper.get('[data-testid="unlock-status"]').attributes("aria-live")).toBe("polite");
    wrapper.unmount();
  });

  it("explains pending keychain setup without removing the password fallback", async () => {
    store.status = { ...createLockedStatus(false), biometricEnabled: true, biometricSetupError: "A signed Mac app with a provisioning profile is required. Your master password still unlocks Hisar." };
    const wrapper = mount(UnlockView);
    await flushPromises();
    expect(wrapper.get('[data-testid="biometric-setup-error"]').text()).toContain("provisioning profile");
    expect(wrapper.find('input[name="password"]').exists()).toBe(true);
    expect(api.unlockWithBiometric).not.toHaveBeenCalled();
  });

  it("keeps pending protection visible after password unlock and never announces activation", async () => {
    const pending = { ...createStatus("unlocked"), biometricAvailable: true, biometricEnabled: true, biometricSetupError: "A macOS provisioning profile is required." };
    api.enableBiometricUnlock.mockResolvedValue(pending);
    store.noticeMessage = "";
    await storeModule.enableBiometricUnlock("master password");
    expect(store.noticeMessage).toBe("");
    const wrapper = mount(MainView);
    expect(wrapper.get('[data-testid="biometric-setup-error"]').text()).toContain("provisioning profile");
    wrapper.unmount();
  });
});

describe("SettingsPanel", () => {
  it("asks for the master password before turning on Touch ID", async () => {
    store.status = { ...createStatus("unlocked"), biometricAvailable: true, biometricEnabled: false, biometricReady: false };
    api.enableBiometricUnlock.mockResolvedValue({ ...store.status, biometricEnabled: true });
    const wrapper = mount(SettingsPanel);
    await wrapper.find('[data-testid="enable-touch-id"]').trigger("click");
    await wrapper.find('input[name="touchIDPassword"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.enableBiometricUnlock).toHaveBeenCalledWith("master password");
    expect(store.status.biometricEnabled).toBe(true);
    expect(wrapper.find('[data-testid="disable-touch-id"]').exists()).toBe(true);
  });

  it("changes the auto-lock delay", async () => {
    store.status = createStatus("unlocked");
    api.setAutoLock.mockResolvedValue({ ...store.status, autoLockSeconds: 30 });
    const wrapper = mount(SettingsPanel);
    const select = wrapper.find('[data-testid="auto-lock"]');
    expect((select.element as HTMLSelectElement).value).toBe("600");
    expect(select.findAll("option").map((option) => option.text())).toEqual(["30 seconds", "1 minute", "5 minutes", "10 minutes", "1 hour"]);
    expect(select.element.closest(".setting")?.textContent).toContain("Hisar locks after this long without keyboard, mouse or trackpad input on this Mac, and when the screen locks.");
    expect(select.element.closest(".setting")?.textContent).not.toContain("Copied values");
    const clipboard = wrapper.get('[data-testid="clipboard-clearing"]');
    expect(clipboard.text()).toBe("Clear copied values after 90 seconds");
    expect(clipboard.find("input, select, button").exists()).toBe(false);
    await select.setValue("30");
    await flushPromises();
    expect(api.setAutoLock).toHaveBeenCalledWith(30);
    expect(store.status.autoLockSeconds).toBe(30);
  });

  it("turns Touch ID off", async () => {
    store.status = { ...createStatus("unlocked"), biometricAvailable: true, biometricEnabled: true, biometricReady: false };
    api.disableBiometricUnlock.mockResolvedValue({ ...store.status, biometricEnabled: false });
    const wrapper = mount(SettingsPanel);
    await wrapper.find('[data-testid="disable-touch-id"]').trigger("click");
    await flushPromises();
    expect(store.status.biometricEnabled).toBe(false);
  });

  it("disables the toggle when Touch ID is unavailable", () => {
    store.status = { ...createStatus("unlocked"), biometricAvailable: false, biometricEnabled: false, biometricReady: false };
    const wrapper = mount(SettingsPanel);
    expect(wrapper.find('[data-testid="enable-touch-id"]').attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("not available on this Mac");
  });
});
