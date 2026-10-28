import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mount, type VueWrapper } from "@vue/test-utils";
import { store } from "./store";
import { createEmergencyKit, createStatus } from "./testing";
import PendingRecoveryReminder from "./components/PendingRecoveryReminder.vue";

vi.mock("./api", async () => {
  // The mock factory runs before static imports initialize the test helpers.
  const { createApiMock } = await import("./testing");
  return { api: createApiMock(), getErrorMessage: String };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: vi.fn(() => vi.fn()) } }));
let wrapper: VueWrapper | undefined;
beforeEach(() => {
  Object.assign(store, { status: createStatus("locked"), isRecoveryOpen: false, onboardingStep: "", pendingRecovery: [{ operationId: "one", kind: "signup", email: "ada@example.com", phase: "delivery-pending" }] });
});
afterEach(() => wrapper?.unmount());
it("names the next step without resuming or switching accounts", async () => {
  wrapper = mount(PendingRecoveryReminder);
  expect(wrapper.text()).toContain("Save your Emergency Kit to finish setting up ada@example.com.");
  expect(wrapper.get("button").attributes("aria-description")).toBe("Opens the Emergency Kit step");
  const account = { ...store.status };
  await wrapper.get("button").trigger("click");
  expect(store.isRecoveryOpen).toBe(true);
  expect(store.status).toEqual(account);
  expect(wrapper.find("button").exists()).toBe(false);
});
it("distinguishes manual sign-in and several records", async () => {
  store.pendingRecovery[0].phase = "manual-unlock";
  wrapper = mount(PendingRecoveryReminder);
  expect(wrapper.text()).toContain("Sign in with your saved Emergency Kit.");
  store.pendingRecovery.push({ ...store.pendingRecovery[0], operationId: "two" });
  await wrapper.vm.$nextTick();
  expect(wrapper.text()).toContain("2 account setups need attention");
  expect(wrapper.get("button").attributes("aria-description")).toBe("Opens the account setup chooser");
});

it("leaves an owed kit to the list banner and the lock screen's static line", () => {
  store.status.emergencyKit = createEmergencyKit({ operationId: "one" });
  wrapper = mount(PendingRecoveryReminder);
  expect(wrapper.find("button").exists()).toBe(false);
});
