import { beforeEach, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import MissingAgentKeyNotice from "./MissingAgentKeyNotice.vue";

const mocks = vi.hoisted(() => ({ list: vi.fn(), openSettings: vi.fn(), listeners: {} as Record<string, () => void> }));
vi.mock("../api", () => ({ api: { listMissingAgentKeys: mocks.list } }));
vi.mock("../store", () => ({ openSettings: mocks.openSettings }));
vi.mock("@wailsio/runtime", () => ({ Events: { On: (name: string, listener: () => void) => {
  mocks.listeners[name] = listener;
  return () => delete mocks.listeners[name];
} } }));

beforeEach(() => {
  vi.clearAllMocks();
  mocks.list.mockResolvedValue([{ sessionID: 1, agentName: "omp" }]);
});

it("keeps an actionable notice without relying on native notifications and clears a resolved session", async () => {
  const wrapper = mount(MissingAgentKeyNotice);
  await flushPromises();
  expect(wrapper.get('[role="status"]').text()).toContain("omp asked for an SSH key.");
  expect(wrapper.text()).toContain("No SSH key is in a vault available to coding agents.");
  await wrapper.get("button").trigger("click");
  expect(mocks.openSettings).toHaveBeenCalledWith("developer");
  mocks.list.mockResolvedValue([]);
  mocks.listeners["missing-agent-keys-changed"]();
  await flushPromises();
  expect(wrapper.find("aside").exists()).toBe(false);
  wrapper.unmount();
  expect(mocks.listeners["missing-agent-keys-changed"]).toBeUndefined();
});

it("does not restore an old notice after a newer session update", async () => {
  const { promise, resolve: finishOld } = Promise.withResolvers<{ sessionID: number; agentName: string }[]>();
  mocks.list.mockReturnValueOnce(promise);
  const wrapper = mount(MissingAgentKeyNotice);
  mocks.list.mockResolvedValue([]);
  mocks.listeners["agent-sessions-changed"]();
  await flushPromises();
  finishOld([{ sessionID: 1, agentName: "omp" }]);
  await flushPromises();
  expect(wrapper.find("aside").exists()).toBe(false);
  wrapper.unmount();
});
