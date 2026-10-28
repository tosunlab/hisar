import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mount, type VueWrapper } from "@vue/test-utils";
import { createEmergencyKit, createStatus } from "./testing";
import { store } from "./store";
import UnlockView from "./components/UnlockView.vue";

vi.mock("./api", async () => {
  // The mock factory runs before static imports initialize the test helpers.
  const { createApiMock } = await import("./testing");
  return { api: createApiMock(), getErrorMessage: String };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: vi.fn(() => vi.fn()) } }));
let wrapper: VueWrapper | undefined;
beforeEach(() => {
  Object.assign(store, { status: createStatus("locked"), errorMessage: "", allShareAwaitingUnlock: [], interruptedKitOperationID: "", pendingRecovery: [] });
});
afterEach(() => wrapper?.unmount());

it("explains why enabled Touch ID needs a password without calling it disabled", () => {
  Object.assign(store.status, { biometricEnabled: true, biometricAvailable: true, biometricReady: false });
  wrapper = mount(UnlockView);
  expect(wrapper.get('[data-testid="biometric-password-reason"]').text()).toContain("after starting and at least every 14 days");
});

it("distinguishes unavailable hardware and keeps setup errors authoritative", async () => {
  Object.assign(store.status, { biometricEnabled: true, biometricAvailable: false, biometricReady: false });
  wrapper = mount(UnlockView);
  expect(wrapper.get('[data-testid="biometric-password-reason"]').text()).toContain("isn't available right now");
  store.status.biometricSetupError = "Unlock to finish setting up Touch ID.";
  await wrapper.vm.$nextTick();
  expect(wrapper.find('[data-testid="biometric-password-reason"]').exists()).toBe(false);
  expect(wrapper.get('[data-testid="biometric-setup-error"]').text()).toBe(store.status.biometricSetupError);
});

it("does not explain password intervals when Touch ID is turned off", () => {
  store.status.biometricEnabled = false;
  wrapper = mount(UnlockView);
  expect(wrapper.find('[data-testid="biometric-password-reason"]').exists()).toBe(false);
});

it("keeps an owed kit below the account without replacing the password unlock", () => {
  store.status.emergencyKit = createEmergencyKit({ isAuthorized: false });
  wrapper = mount(UnlockView);
  expect(wrapper.get('[data-testid="locked-kit-reminder"]').text()).toBe("Your Emergency Kit isn't saved yet. Unlock Hisar to save it.");
  expect(wrapper.get('input[name="password"]').attributes("type")).toBe("password");
  expect(wrapper.find('[data-testid="locked-kit-reminder"] button').exists()).toBe(false);
});

it("distinguishes an interrupted kit and a saved copy awaiting confirmation", async () => {
  store.status.emergencyKit = createEmergencyKit({ savedBy: "file", isAuthorized: false });
  wrapper = mount(UnlockView);
  expect(wrapper.get('[data-testid="locked-kit-reminder"]').text()).toBe("Your Emergency Kit needs confirmation. Unlock Hisar to continue.");
  store.interruptedKitOperationID = store.status.emergencyKit.operationId;
  await wrapper.vm.$nextTick();
  expect(wrapper.get('[data-testid="locked-kit-reminder"]').text()).toBe("Hisar locked while you were saving your Emergency Kit. Unlock to continue.");
});

it("opens lost-key help without erasing the typed unlock password", async () => {
  wrapper = mount(UnlockView, { attachTo: document.body });
  await wrapper.get('input[name="password"]').setValue("still typed");
  await wrapper.findAll("button").find(button => button.text() === "Can't find your Secret Key?")!.trigger("click");
  expect(wrapper.text()).toContain("If you saved the Secret Key separately");
  await wrapper.findAll("button").find(button => button.text() === "Done")!.trigger("click");
  expect((wrapper.get('input[name="password"]').element as HTMLInputElement).value).toBe("still typed");
});

it("makes Cancel the default for sign-out and preserves the full loss disclosure", async () => {
  store.status.emergencyKit = createEmergencyKit({ isAuthorized: false });
  wrapper = mount(UnlockView, { attachTo: document.body });
  await wrapper.findAll("button").find(button => button.text() === "Sign out of this device")!.trigger("click");
  expect(document.activeElement?.textContent).toBe("Cancel");
  const confirmation = wrapper.get(".confirm").text();
  expect(confirmation).toContain("master password");
  expect(confirmation).toContain("offline");
  expect(confirmation).toContain("protected");
  expect(confirmation).toContain("unlock Hisar");
  await wrapper.get(".confirm").trigger("keydown", { key: "Escape" });
  expect(wrapper.find(".confirm").exists()).toBe(false);
});
