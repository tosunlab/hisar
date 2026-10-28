import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createApiMock, createEmergencyKit, createStatus } from "../testing";
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
enableAutoUnmount(afterEach);

beforeEach(() => {
  vi.clearAllMocks();
  api.getSSHAgentStatus?.mockResolvedValue?.({});
  Object.assign(store, {
    status: { ...createStatus("unlocked"), biometricAvailable: false, biometricEnabled: false, biometricReady: false },
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    onboardingStep: "",
  });
});

describe("Emergency Kit after sign up", () => {
  beforeEach(() => {
    store.status.emergencyKit = createEmergencyKit();
    store.onboardingStep = "kit";
    api.getStatus.mockImplementation(async () => ({ ...store.status }));
  });

  it("shows the key without asking again and explains keeping an off-device copy", async () => {
    const wrapper = mount(SecretKeyView);
    await flushPromises();
    expect(wrapper.get('[data-testid="secret-key"]').attributes("aria-label")).toBe("H1-ABC123-SECRET");
    expect(wrapper.find('input[type="password"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("Keep it where you can get to it without this Mac");
    expect(wrapper.text()).not.toContain("Hisar keeps a copy on this Mac.");
    expect(api.authorizeNewAccountEmergencyKit).not.toHaveBeenCalled();
  });

  it("requires a real save and then only one acknowledgement action", async () => {
    api.saveNewAccountEmergencyKit.mockImplementation(async () => {
      store.status.emergencyKit = createEmergencyKit({ savedBy: "file" });
      return "/Users/alper/Documents/Hisar Emergency Kit.pdf";
    });
    const wrapper = mount(SecretKeyView);
    expect(wrapper.find('[data-testid="secret-key-continue"]').exists()).toBe(false);
    await wrapper.get('[data-testid="save-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false);
    expect(wrapper.get('[role="status"]').text()).toBe("Emergency Kit saved.");
    expect(wrapper.get('[data-testid="secret-key-continue"]').text()).toBe("I saved my Emergency Kit");
    api.getStatus.mockResolvedValue({ ...createStatus("unlocked"), biometricOfferAvailable: true });
    await wrapper.get('[data-testid="secret-key-continue"]').trigger("click");
    await flushPromises();
    expect(api.acknowledgeNewAccountRecovery).toHaveBeenCalledTimes(1);
    expect(store.status.state).toBe("unlocked");
    expect(store.onboardingStep).toBe("biometric");
  });

  it("does not count Copy or a cancelled save", async () => {
    api.saveNewAccountEmergencyKit.mockResolvedValue("");
    const wrapper = mount(SecretKeyView);
    await flushPromises();
    await wrapper.findAll("button").find(button => button.text() === "Copy Secret Key")!.trigger("click");
    await wrapper.get('[data-testid="save-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("The Emergency Kit wasn't saved.");
    expect(wrapper.find('[data-testid="secret-key-continue"]').exists()).toBe(false);
    expect(api.acknowledgeNewAccountRecovery).not.toHaveBeenCalled();
  });

  it("offers Touch ID after Save later without dropping the kit authorization", async () => {
    store.status.biometricOfferAvailable = true;
    const wrapper = mount(SecretKeyView);
    await wrapper.get('[data-testid="secret-key-later"]').trigger("click");
    await flushPromises();
    expect(api.postponeNewAccountEmergencyKit).toHaveBeenCalledTimes(1);
    expect(api.forgetNewAccountSecretKey).not.toHaveBeenCalled();
    expect(store.status.emergencyKit?.isAuthorized).toBe(true);
    expect(store.onboardingStep).toBe("biometric");
  });

  it("asks once with Touch ID after a biometric unlock", async () => {
    store.status.biometricEnabled = true;
    store.status.emergencyKit = createEmergencyKit({ isAuthorized: false });
    api.authorizeNewAccountEmergencyKitWithBiometric.mockImplementation(async () => {
      store.status.emergencyKit = createEmergencyKit();
    });
    const wrapper = mount(SecretKeyView);
    expect(wrapper.find('[data-testid="secret-key"]').exists()).toBe(false);
    await wrapper.findAll("button").find(button => button.text() === "Continue with Touch ID")!.trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="secret-key"]').exists()).toBe(true);
    expect(api.authorizeNewAccountEmergencyKitWithBiometric).toHaveBeenCalledTimes(1);
    expect(api.authorizeNewAccountEmergencyKit).not.toHaveBeenCalled();
  });

  it("retains the gate on a wrong password and closes it without postponing", async () => {
    store.status.emergencyKit = createEmergencyKit({ isAuthorized: false });
    api.authorizeNewAccountEmergencyKit.mockRejectedValueOnce(new Error("The master password is incorrect."));
    const wrapper = mount(SecretKeyView);
    await wrapper.get('input[name="kitPassword"]').setValue("wrong password");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toBe("The master password is incorrect.");
    await wrapper.findAll("button").find(button => button.text() === "Cancel")!.trigger("click");
    expect(store.onboardingStep).toBe("");
    expect(api.postponeNewAccountEmergencyKit).not.toHaveBeenCalled();
  });

  it("does not show a reveal that completes after a lock", async () => {
    const reveal = Promise.withResolvers<{ secretKey: string; qrPayload: string; qrCode: string }>();
    api.revealNewAccountSecretKey.mockReturnValueOnce(reveal.promise);
    const wrapper = mount(SecretKeyView);
    store.recoveryEpoch++;
    store.status = { ...store.status, state: "locked", emergencyKit: createEmergencyKit({ isAuthorized: false }) };
    await wrapper.vm.$nextTick();
    reveal.resolve({ secretKey: "H1-TOO-LATE", qrPayload: "", qrCode: "" });
    await flushPromises();
    expect(wrapper.text()).not.toContain("H1-TOO-LATE");
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
