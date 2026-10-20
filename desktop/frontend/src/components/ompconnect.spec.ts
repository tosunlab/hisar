import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { api as mockedApi } from "../api";
import { store } from "../store";
import { createStatus } from "../testing";
import CodingAgentsSection from "./CodingAgentsSection.vue";

// vi.mock runs before the imports above, so its factory loads the mock itself.
vi.mock("../api", async () => {
  const { createApiMock } = await import("../testing");
  return {
    api: createApiMock(),
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
  };
});

const api = vi.mocked(mockedApi);

async function mountSection(): Promise<VueWrapper> {
  const wrapper = mount(CodingAgentsSection, { attachTo: document.body });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    isBusy: false,
    allVault: [{ id: "work", name: "Work", type: "shared", role: "editor", itemCount: 3 }],
    allAgentVaultID: [],
    agentErrorMessage: "",
  });
  api.listAllAgentVaultID.mockResolvedValue(["work"]);
});

describe("OmpConnectCard", () => {
  it("says omp is not installed and offers no Connect", async () => {
    api.getOmpState.mockResolvedValue("not-installed");
    const wrapper = await mountSection();
    expect(wrapper.find('[data-testid="omp-state"]').text()).toBe("omp is not installed. Install omp, or Tern, which comes with it.");
    expect(wrapper.find('[data-testid="connect-omp"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="disconnect-omp"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("connects omp and asks to restart running sessions", async () => {
    api.getOmpState.mockResolvedValue("available");
    const wrapper = await mountSection();
    expect(wrapper.find('[data-testid="omp-state"]').text()).toContain("hisar run");
    expect(wrapper.find('[data-testid="omp-restart-hint"]').exists()).toBe(false);

    api.connectOmp.mockResolvedValue("connected");
    await wrapper.find('[data-testid="connect-omp"]').trigger("click");
    await flushPromises();
    expect(api.connectOmp).toHaveBeenCalledTimes(1);
    expect(wrapper.find('[data-testid="omp-state"]').text()).toBe("Connected");
    expect(wrapper.find('[data-testid="omp-restart-hint"]').text()).toBe("New omp sessions load Hisar. Restart the ones already running.");
    expect(wrapper.find('[data-testid="connect-omp"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("disconnects a connected omp", async () => {
    api.getOmpState.mockResolvedValue("connected");
    const wrapper = await mountSection();
    api.disconnectOmp.mockResolvedValue("available");
    await wrapper.find('[data-testid="disconnect-omp"]').trigger("click");
    await flushPromises();
    expect(api.disconnectOmp).toHaveBeenCalledTimes(1);
    expect(wrapper.find('[data-testid="connect-omp"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="omp-restart-hint"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows why omp could not connect and keeps Connect", async () => {
    api.getOmpState.mockResolvedValue("available");
    const wrapper = await mountSection();
    api.connectOmp.mockRejectedValue(new Error("omp could not connect to Hisar: package.json not found"));
    await wrapper.find('[data-testid="connect-omp"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="omp-error"]').text()).toBe("omp could not connect to Hisar: package.json not found");
    expect(wrapper.find('[data-testid="connect-omp"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("links to the vault switches while no vault is available to agents", async () => {
    api.getOmpState.mockResolvedValue("connected");
    api.listAllAgentVaultID.mockResolvedValue([]);
    const wrapper = await mountSection();
    expect(wrapper.find('[data-testid="omp-no-vaults"]').text()).toContain("omp sees no secrets until a vault is available to agents.");
    await wrapper.find('[data-testid="show-agent-vaults"]').trigger("click");
    expect(document.activeElement).toBe(wrapper.find('input[name="agentVault-work"]').element);

    api.setVaultAvailableToAgents.mockResolvedValue(["work"]);
    await wrapper.find('input[name="agentVault-work"]').setValue(true);
    await flushPromises();
    expect(wrapper.find('[data-testid="omp-no-vaults"]').exists()).toBe(false);
    wrapper.unmount();
  });
});
