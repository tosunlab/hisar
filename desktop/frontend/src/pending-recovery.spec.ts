import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mount, type VueWrapper } from "@vue/test-utils";
import { handleLocked, loadStatus, openEmergencyKit, store, unlock, unlockWithBiometric } from "./store";
import { api, type PendingRecoverySummary } from "./api";
import { createEmergencyKit, createStatus } from "./testing";
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
beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { status: createStatus("signed-out"), onboardingStep: "", interruptedKitOperationID: "", pendingRecovery: [record("one")], isRecoveryOpen: false, recoveryFactor: null, isBusy: false, errorMessage: "" });
  vi.mocked(api.listAllVault).mockResolvedValue([]);
});
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
  await wrapper.findAll('[data-testid="pending-recovery-chooser"] button')[0]!.trigger("click");
  expect(wrapper.get("article .primary").text()).toBe("Continue");
  await wrapper.get('input[name="pendingRecoveryPassword"]').setValue("secret");
  await wrapper.findAll('[data-testid="pending-recovery-chooser"] button')[1]!.trigger("click");
  expect((wrapper.get('input[name="pendingRecoveryPassword"]').element as HTMLInputElement).value).toBe("");
  expect(wrapper.get("article .primary").text()).toBe("Sign in with saved Emergency Kit");
});

it("never opens Finish account setup for the installed account's owed kit on launch", async () => {
  vi.mocked(api.getStatus).mockResolvedValue({ ...createStatus("locked"), emergencyKit: createEmergencyKit({ operationId: "one", isAuthorized: false }) });
  vi.mocked(api.listPendingRecovery).mockResolvedValue([record("one")]);
  await loadStatus();
  expect(store.status.state).toBe("locked");
  expect(store.isRecoveryOpen).toBe(false);
  expect(store.onboardingStep).toBe("");
  vi.mocked(api.unlock).mockResolvedValue({ ...createStatus("unlocked"), emergencyKit: createEmergencyKit({ operationId: "one" }) });
  await unlock("master password");
  expect(store.onboardingStep).toBe("");
  openEmergencyKit();
  expect(store.onboardingStep).toBe("kit");
  expect(api.authorizeNewAccountEmergencyKit).not.toHaveBeenCalled();
});

it.each(["password", "biometric"])("resumes an interrupted kit after %s unlock, then forgets the intent", async method => {
  store.status = { ...createStatus("unlocked"), emergencyKit: createEmergencyKit({ operationId: "one" }) };
  openEmergencyKit();
  vi.mocked(api.getStatus).mockResolvedValue({ ...store.status, state: "locked", emergencyKit: createEmergencyKit({ operationId: "one", isAuthorized: false }) });
  await handleLocked();
  expect(store.onboardingStep).toBe("");
  expect(store.interruptedKitOperationID).toBe("one");
  const unlocked = { ...createStatus("unlocked"), emergencyKit: createEmergencyKit({ operationId: "one", isAuthorized: method === "password" }) };
  vi.mocked(api.unlock).mockResolvedValue(unlocked);
  vi.mocked(api.unlockWithBiometric).mockResolvedValue(unlocked);
  if (method === "password") await unlock("master password");
  else await unlockWithBiometric();
  expect(store.onboardingStep).toBe("kit");
  expect(store.status.emergencyKit?.isAuthorized).toBe(method === "password");
  expect(store.interruptedKitOperationID).toBe("");
});

it("drops interrupted-kit intent when the account changes", async () => {
  store.status = { ...createStatus("locked"), emergencyKit: createEmergencyKit({ operationId: "one" }) };
  store.interruptedKitOperationID = "one";
  vi.mocked(api.unlock).mockResolvedValue({ ...createStatus("unlocked"), email: "other@example.com", emergencyKit: createEmergencyKit({ operationId: "two" }) });
  await unlock("master password");
  expect(store.onboardingStep).toBe("");
  expect(store.interruptedKitOperationID).toBe("");
});
