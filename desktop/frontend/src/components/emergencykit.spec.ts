import iosSettingsSource from "../../../../mobile/ios/App/Views/SettingsView.swift?raw";
import androidAccountSource from "../../../../mobile/android/app/src/main/kotlin/com/tryhisar/hisar/ui/settings/AccountScreens.kt?raw";
import androidNewKeySource from "../../../../mobile/android/app/src/main/kotlin/com/tryhisar/hisar/ui/signin/NewSecretKeyScreen.kt?raw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createApiMock, createStatus } from "../testing";
import SecuritySection from "./SecuritySection.vue";

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

const { store } = await import("../store");
const SecretKeyView = (await import("./SecretKeyView.vue")).default;
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  vi.clearAllMocks();
  api.getSSHAgentStatus?.mockResolvedValue?.({});
  Object.assign(store, {
    status: { ...createStatus("unlocked"), biometricAvailable: false, biometricEnabled: false, biometricReady: false },
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    pendingSecretKey: "H1-ABC123-SECRET",
  });
});

describe("Emergency Kit after sign up", () => {
  it("describes device copies without claiming exclusive possession on every key screen", () => {
    const wrapper = mount(SecretKeyView);
    expect(wrapper.text()).toContain("Hisar keeps a copy on this Mac. The Hisar server does not have it. Anyone with your Secret Key and master password can open your vaults.");
    for (const [source, device] of [
      [iosSettingsSource, "this iPhone"],
      [androidAccountSource, "this device"],
      [androidNewKeySource, "this device"],
    ]) {
      expect(source).toContain(`Hisar keeps a copy on ${device}. The Hisar server does not have it. Anyone with your Secret Key and master password can open your vaults.`);
      expect(source).not.toContain("nobody else has a copy");
    }
  });

  it("saves the PDF and tells the user where", async () => {
    api.saveNewAccountEmergencyKit.mockResolvedValue("/Users/alper/Documents/Hisar Emergency Kit.pdf");
    const wrapper = mount(SecretKeyView);
    expect(wrapper.get('[data-testid="save-emergency-kit"]').text()).toBe("Save Emergency Kit…");
    await wrapper.find('[data-testid="save-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(api.saveNewAccountEmergencyKit).toHaveBeenCalled();
    expect(store.noticeMessage).toBe("Emergency Kit saved to Hisar Emergency Kit.pdf");
  });

  it("shows nothing when the save dialog is cancelled", async () => {
    api.saveNewAccountEmergencyKit.mockResolvedValue("");
    const wrapper = mount(SecretKeyView);
    await wrapper.find('[data-testid="save-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(store.noticeMessage).toBe("");
    expect(store.errorMessage).toBe("");
  });

  it("forgets the Secret Key in the backend once the user confirms", async () => {
    const wrapper = mount(SecretKeyView);
    await wrapper.find('input[name="hasSaved"]').setValue(true);
    await wrapper.findAll("button").find((button) => button.text() === "Continue to Hisar")!.trigger("click");
    expect(store.pendingSecretKey).toBe("");
    expect(api.forgetNewAccountSecretKey).toHaveBeenCalled();
  });
});

describe("Emergency Kit from Settings", () => {
  it("asks for the master password before saving", async () => {
    api.saveEmergencyKit.mockResolvedValue("/tmp/Hisar Emergency Kit.pdf");
    const wrapper = mount(SettingsPanel);
    expect(wrapper.get('[data-testid="save-emergency-kit"]').text()).toBe("Save Emergency Kit…");
    await wrapper.find('[data-testid="save-emergency-kit"]').trigger("click");
    expect(api.saveEmergencyKit).not.toHaveBeenCalled();
    const emergencyKitForm = wrapper.get('input[name="emergencyKitPassword"]').element.closest("form")!;
    expect(emergencyKitForm.querySelector('button[type="submit"]')?.textContent).toBe("Save Emergency Kit…");
    await wrapper.find('input[name="emergencyKitPassword"]').setValue("master password");
    await (wrapper.find('input[name="emergencyKitPassword"]').element as HTMLInputElement).form!.dispatchEvent(new Event("submit"));
    await flushPromises();
    expect(api.saveEmergencyKit).toHaveBeenCalledWith("master password");
    expect(store.noticeMessage).toContain("Emergency Kit saved");
    expect(wrapper.find('input[name="emergencyKitPassword"]').exists()).toBe(false);
  });

  it("shows the error for a wrong master password", async () => {
    api.saveEmergencyKit.mockRejectedValue(new Error("incorrect email, master password or Secret Key"));
    const wrapper = mount(SettingsPanel);
    await wrapper.find('[data-testid="save-emergency-kit"]').trigger("click");
    await wrapper.find('input[name="emergencyKitPassword"]').setValue("wrong");
    await (wrapper.find('input[name="emergencyKitPassword"]').element as HTMLInputElement).form!.dispatchEvent(new Event("submit"));
    await flushPromises();
    expect(store.errorMessage).toContain("incorrect");
    expect(wrapper.find('input[name="emergencyKitPassword"]').exists()).toBe(true);
  });
});

describe("Secret Key from Recovery", () => {
  const revealed = { secretKey: "H1-ABC123-SECRET", qrPayload: "HISARKIT1 payload", qrCode: "png" };

  async function submit(wrapper: VueWrapper): Promise<void> {
    await wrapper.get('[data-testid="show-secret-key"]').trigger("click");
    await wrapper.get('input[name="revealSecretKeyPassword"]').setValue("master password");
    await wrapper.get('[data-testid="reveal-secret-key-form"]').trigger("submit");
  }

  it("verifies before showing a key and QR, and copies through the backend", async () => {
    api.revealSecretKey.mockResolvedValue(revealed);
    api.copyText.mockResolvedValue(undefined);
    const wrapper = mount(SecuritySection);
    expect(wrapper.find('[data-testid="revealed-secret-key"]').exists()).toBe(false);
    await submit(wrapper);
    await flushPromises();
    expect(api.revealSecretKey).toHaveBeenCalledWith("master password");
    expect(api.saveEmergencyKit).not.toHaveBeenCalled();
    expect(wrapper.get('[data-testid="secret-key-qr"]').attributes("src")).toBe("data:image/png;base64,png");
    await wrapper.get('[data-testid="copy-secret-key"]').trigger("click");
    await flushPromises();
    expect(api.copyText).toHaveBeenCalledWith(revealed.secretKey);
    expect(store.noticeMessage).toBe("Secret Key copied. It clears in 90 seconds.");
    wrapper.unmount();
  });

  it.each(["leave", "lock", "account", "unmount"])("discards a pending key after %s", async (boundary) => {
    const { promise, resolve } = Promise.withResolvers<typeof revealed>();
    api.revealSecretKey.mockReturnValue(promise);
    const wrapper = mount(SecuritySection);
    await submit(wrapper);
    if (boundary === "leave") await wrapper.setProps({ isActive: false });
    if (boundary === "lock") store.status.state = "locked";
    if (boundary === "account") store.status.email = "other@example.com";
    if (boundary === "unmount") wrapper.unmount();
    resolve(revealed);
    await flushPromises();
    if (boundary !== "unmount") {
      expect(wrapper.find('[data-testid="revealed-secret-key"]').exists()).toBe(false);
      expect(wrapper.find('input[name="revealSecretKeyPassword"]').exists()).toBe(false);
      wrapper.unmount();
    }
  });

  it("hides a revealed key when leaving Recovery", async () => {
    api.revealSecretKey.mockResolvedValue(revealed);
    const wrapper = mount(SecuritySection);
    await submit(wrapper);
    await flushPromises();
    await wrapper.setProps({ isActive: false });
    await wrapper.setProps({ isActive: true });
    expect(wrapper.find('[data-testid="revealed-secret-key"]').exists()).toBe(false);
    wrapper.unmount();
  });
});
