import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createApiMock, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] }),
  };
});

const { store } = await import("../store");
const CodingAgentsSection = (await import("./CodingAgentsSection.vue")).default;
const api = apiMock.current;

const allVault = [
  { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 12 },
  { id: "work", name: "Work", type: "shared", role: "editor", itemCount: 1 },
];

function getSwitch(wrapper: ReturnType<typeof mount>, vaultID: string): HTMLInputElement {
  return wrapper.find(`input[name="agentVault-${vaultID}"]`).element as HTMLInputElement;
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    allVault,
    allAgentVaultID: [],
    agentErrorMessage: "",
  });
  api.listAllAgentVaultID.mockResolvedValue([]);
});

describe("CodingAgentsSection", () => {
  it("lists every vault as invisible to agents by default and shows how to add hisar mcp", async () => {
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect(api.listAllAgentVaultID).toHaveBeenCalledTimes(1);
    expect(wrapper.find('[data-testid="coding-agents-snippet"]').text()).toBe("claude mcp add hisar -- hisar mcp");
    expect(wrapper.find('[data-testid="coding-agents-explanation"]').text()).toBe(
      "Agents can find items in these vaults and use their secrets after you approve; other vaults stay invisible to them.",
    );
    expect(getSwitch(wrapper, "personal").checked).toBe(false);
    expect(getSwitch(wrapper, "work").checked).toBe(false);
    expect(wrapper.find('[data-testid="agent-vault-personal"]').text()).toContain("12 items · Invisible to agents");
    expect(wrapper.find('[data-testid="agent-vault-work"]').text()).toContain("1 item · Invisible to agents");
  });

  it("shows the saved setting and turns a vault on and off", async () => {
    api.listAllAgentVaultID.mockResolvedValue(["work"]);
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect(getSwitch(wrapper, "work").checked).toBe(true);
    expect(wrapper.find('[data-testid="agent-vault-work"]').text()).toContain("Available to agents");

    api.setVaultAvailableToAgents.mockResolvedValue(["work", "personal"]);
    await wrapper.find('input[name="agentVault-personal"]').setValue(true);
    await flushPromises();
    expect(api.setVaultAvailableToAgents).toHaveBeenCalledWith("personal", true);
    expect(store.noticeMessage).toBe("Coding agents can use Personal after you approve");
    expect(getSwitch(wrapper, "personal").checked).toBe(true);

    api.setVaultAvailableToAgents.mockResolvedValue(["personal"]);
    await wrapper.find('input[name="agentVault-work"]').setValue(false);
    await flushPromises();
    expect(api.setVaultAvailableToAgents).toHaveBeenLastCalledWith("work", false);
    expect(store.noticeMessage).toBe("Work is invisible to coding agents now");
    expect(getSwitch(wrapper, "work").checked).toBe(false);
  });

  it("shows why saving failed and keeps the saved setting", async () => {
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    api.setVaultAvailableToAgents.mockRejectedValue(new Error("Hisar is locked"));
    await wrapper.find('input[name="agentVault-personal"]').setValue(true);
    await flushPromises();
    expect(wrapper.find('[data-testid="coding-agents-error"]').text()).toBe("Hisar is locked");
    expect(getSwitch(wrapper, "personal").checked).toBe(false);
    expect(store.errorMessage).toBe("");
  });
});
