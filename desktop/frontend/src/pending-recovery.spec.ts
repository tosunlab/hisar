import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mount, type VueWrapper } from "@vue/test-utils";
import { store } from "./store";
import type { PendingRecoverySummary } from "./api";
import PendingRecoveryView from "./components/PendingRecoveryView.vue";

vi.mock("./api", async () => {
  // The mock factory runs before static imports initialize the test helpers.
  const { createApiMock } = await import("./testing");
  return { api: createApiMock(), getErrorMessage: String };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: vi.fn(() => vi.fn()) } }));
const record = (operationId: string, phase = "delivery-pending"): PendingRecoverySummary => ({
  operationId, phase, kind: "signup", email: `${operationId}@example.com`, origin: "https://hisar.test", accountId: operationId,
  name: operationId, kitSaved: false, problems: [], manualUnlock: false, keychainProtected: true, revision: 1,
});
let wrapper: VueWrapper | undefined;
beforeEach(() => Object.assign(store, { pendingRecovery: [record("one")], recoveryFactor: null, isBusy: false, errorMessage: "" }));
afterEach(() => wrapper?.unmount());
it("selects a single record and puts its password immediately before the next action", async () => {
  wrapper = mount(PendingRecoveryView);
  expect(wrapper.find('[data-testid="pending-recovery-chooser"]').exists()).toBe(false);
  expect(wrapper.get("article .primary").text()).toBe("Save Emergency Kit…");
  expect(wrapper.get("article .primary").attributes("disabled")).toBeDefined();
  expect(wrapper.text()).toContain("Enter your master password to continue.");
  await wrapper.get('input[name="pendingRecoveryPassword"]').setValue("master password");
  expect(wrapper.get("article .primary").attributes("disabled")).toBeUndefined();
});
it("shows Continue for uncertain outcomes and clears factors when a chooser changes operation", async () => {
  store.pendingRecovery = [record("one", "outcome-unknown"), record("two", "manual-unlock")];
  wrapper = mount(PendingRecoveryView);
  await wrapper.get("select").setValue("one");
  expect(wrapper.get("article .primary").text()).toBe("Continue");
  await wrapper.get('input[name="pendingRecoveryPassword"]').setValue("secret");
  await wrapper.get("select").setValue("two");
  expect((wrapper.get('input[name="pendingRecoveryPassword"]').element as HTMLInputElement).value).toBe("");
  expect(wrapper.get("article .primary").text()).toBe("Sign in with saved Emergency Kit");
});
