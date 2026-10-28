import { beforeEach, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import type { PendingRecoverySummary } from "../api";
import type * as Vue from "vue";
import { store, resumePendingRecovery } from "../store";
import PendingRecoveryView from "./PendingRecoveryView.vue";

vi.mock("../store", async () => {
  const { reactive } = await vi.importActual<typeof Vue>("vue");
  const state = reactive({ status: { state: "signed-out", email: "", server: "" }, pendingRecovery: [] as PendingRecoverySummary[], recoveryEpoch: 0, recoveryFactor: null, isBusy: false, errorMessage: "", isRecoveryOpen: true });
  return {
    store: state,
    needsAttentionRecovery: () => state.pendingRecovery,
    resumePendingRecovery: vi.fn(),
    savePendingRecoveryKit: vi.fn(),
    acknowledgePendingRecovery: vi.fn(),
    discardPreparedRecovery: vi.fn(),
  };
});

function recovery(overrides: Partial<PendingRecoverySummary> = {}): PendingRecoverySummary {
  return { operationId: "signup", kind: "signup", origin: "https://hisar.test", accountId: "account", email: "a@example.com", name: "A", phase: "delivery-pending", kitSaved: false, problems: [], manualUnlock: false, keychainProtected: true, revision: 1, ...overrides };
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { pendingRecovery: [recovery()], errorMessage: "", isBusy: false });
});

it("asks only for the password on a protected delivery-pending signup", () => {
  const wrapper = mount(PendingRecoveryView);
  expect(wrapper.findAll('input[name="pendingRecoveryPassword"]')).toHaveLength(1);
  expect(wrapper.find('input[name="pendingRecoverySecretKey"]').exists()).toBe(false);
  expect(wrapper.find('input[name="pendingRecoveryPriorSecretKey"]').exists()).toBe(false);
});

it.each([{ keychainProtected: false }, { manualUnlock: true }])("shows a readable Secret Key only when the operation needs one", (overrides) => {
  store.pendingRecovery = [recovery(overrides)];
  const wrapper = mount(PendingRecoveryView);
  expect(wrapper.get('input[name="pendingRecoverySecretKey"]').attributes()).toMatchObject({ type: "text", class: "monospace", placeholder: "H1-XXXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX" });
});

it("keeps factors local to the selected operation", async () => {
  store.pendingRecovery = [recovery(), recovery({ operationId: "replacement", kind: "replacement", phase: "needs-finishing" })];
  const wrapper = mount(PendingRecoveryView);
  const choices = wrapper.findAll('[data-testid="pending-recovery-chooser"] button');
  await choices[1]!.trigger("click");
  await wrapper.get('input[name="pendingRecoveryPassword"]').setValue("replacement password");
  await choices[0]!.trigger("click");
  expect((wrapper.get('input[name="pendingRecoveryPassword"]').element as HTMLInputElement).value).toBe("");
  await choices[1]!.trigger("click");
  await wrapper.get('input[name="pendingRecoveryPassword"]').setValue("replacement password");
  await wrapper.get("article .primary").trigger("click");
  expect(resumePendingRecovery).toHaveBeenCalledWith("replacement", "replacement password", "", "");
});

it("asks for the previous key only after an interrupted replacement requests it", async () => {
  store.pendingRecovery = [recovery({ operationId: "replacement", kind: "replacement", phase: "needs-finishing" })];
  vi.mocked(resumePendingRecovery).mockImplementation(async () => {
    store.errorMessage = "enter the previous Secret Key to resume this interrupted replacement";
    return false;
  });
  const wrapper = mount(PendingRecoveryView);
  expect(wrapper.find('input[name="pendingRecoveryPriorSecretKey"]').exists()).toBe(false);
  await wrapper.get('input[name="pendingRecoveryPassword"]').setValue("master password");
  await wrapper.get("article .primary").trigger("click");
  await flushPromises();
  expect(wrapper.get('input[name="pendingRecoveryPriorSecretKey"]').attributes("type")).toBe("text");
});
