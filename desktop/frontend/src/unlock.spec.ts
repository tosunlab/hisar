import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mount, type VueWrapper } from "@vue/test-utils";
import { createStatus } from "./testing";
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
  Object.assign(store, { status: createStatus("locked"), errorMessage: "", allShareAwaitingUnlock: [] });
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
