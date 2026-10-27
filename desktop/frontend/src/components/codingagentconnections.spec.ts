import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { api as mockedApi } from "../api";
import { store } from "../store";
import { createCodingAgent, createStatus } from "../testing";
import CodingAgentsSection from "./CodingAgentsSection.vue";
import SettingsPanel from "./SettingsPanel.vue";

// vi.mock runs before the imports above, so its factory loads the mock itself.
vi.mock("../api", async () => {
  const { createApiMock } = await import("../testing");
  return {
    api: createApiMock(),
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
  };
});

const api = vi.mocked(mockedApi);

const omp = createCodingAgent({ id: "omp", name: "omp" });
const claudeCode = createCodingAgent({ id: "claude-code", name: "Claude Code" });
const codex = createCodingAgent({ id: "codex", name: "Codex" });
const hermes = createCodingAgent({ id: "hermes", name: "Hermes" });

async function mountSection(): Promise<VueWrapper> {
  const wrapper = mount(CodingAgentsSection, { attachTo: document.body });
  await flushPromises();
  return wrapper;
}

function getText(wrapper: VueWrapper, testID: string): string {
  return wrapper.find(`[data-testid="${testID}"]`).text();
}

function hasElement(wrapper: VueWrapper, testID: string): boolean {
  return wrapper.find(`[data-testid="${testID}"]`).exists();
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    isBusy: false,
    allVault: [{ id: "work", name: "Work", type: "shared", role: "editor", itemCount: 3 }],
    allAgentVaultID: [],
    agentErrors: { vaults: "", signIn: "", grants: "", sessions: "", hosts: "" },
  });
  api.listAllAgentVaultID.mockResolvedValue(["work"]);
});

describe("CodingAgentConnections", () => {
  it("lists omp, Claude Code and Codex, each with what it can do", async () => {
    api.listAllCodingAgent.mockResolvedValue([
      { ...omp, state: "not-installed" },
      { ...claudeCode, state: "available" },
      { ...codex, state: "connected" },
    ]);
    const wrapper = await mountSection();
    expect(wrapper.findAll('[data-testid^="agent-card-"] strong').map((name) => name.text())).toEqual(["omp", "Claude Code", "Codex"]);

    expect(getText(wrapper, "agent-state-omp")).toBe("omp is not installed. Install omp, or Tern, which comes with it.");
    expect(hasElement(wrapper, "connect-agent-omp")).toBe(false);
    expect(hasElement(wrapper, "disconnect-agent-omp")).toBe(false);

    expect(hasElement(wrapper, "connect-agent-claude-code")).toBe(true);
    expect(hasElement(wrapper, "agent-restart-hint-claude-code")).toBe(false);

    expect(getText(wrapper, "agent-state-codex")).toBe("Connected");
    expect(getText(wrapper, "agent-restart-hint-codex")).toBe("New Codex sessions load Hisar. Restart the ones already running.");
    expect(hasElement(wrapper, "disconnect-agent-codex")).toBe(true);
    wrapper.unmount();
  });

  it("says Codex is not installed and omp is available", async () => {
    api.listAllCodingAgent.mockResolvedValue([omp, { ...codex, state: "not-installed" }]);
    const wrapper = await mountSection();
    expect(getText(wrapper, "agent-state-omp")).toContain("through hisar run and show Hisar's state in their status line.");
    expect(getText(wrapper, "agent-state-codex")).toBe("Codex is not installed.");
    wrapper.unmount();
  });

  it("connects Claude Code and asks to restart running sessions", async () => {
    api.listAllCodingAgent.mockResolvedValue([omp, claudeCode]);
    const wrapper = await mountSection();
    api.connectCodingAgent.mockResolvedValue({ ...claudeCode, state: "connected" });
    await wrapper.find('[data-testid="connect-agent-claude-code"]').trigger("click");
    await flushPromises();
    expect(api.connectCodingAgent).toHaveBeenCalledWith("claude-code");
    expect(getText(wrapper, "agent-state-claude-code")).toBe("Connected");
    expect(getText(wrapper, "agent-restart-hint-claude-code")).toBe("New Claude Code sessions load Hisar. Restart the ones already running.");
    expect(hasElement(wrapper, "connect-agent-claude-code")).toBe(false);
    expect(hasElement(wrapper, "connect-agent-omp")).toBe(true);
    wrapper.unmount();
  });

  it("connects omp and asks to restart running sessions", async () => {
    api.listAllCodingAgent.mockResolvedValue([omp]);
    const wrapper = await mountSection();
    api.connectCodingAgent.mockResolvedValue({ ...omp, state: "connected" });
    await wrapper.find('[data-testid="connect-agent-omp"]').trigger("click");
    await flushPromises();
    expect(getText(wrapper, "agent-restart-hint-omp")).toBe("New omp sessions load Hisar. Restart the ones already running.");
    wrapper.unmount();
  });

  it("connects Hermes and tells how sessions load Hisar", async () => {
    api.listAllCodingAgent.mockResolvedValue([hermes]);
    const wrapper = await mountSection();
    api.connectCodingAgent.mockResolvedValue({ ...hermes, state: "connected" });
    await wrapper.find('[data-testid="connect-agent-hermes"]').trigger("click");
    await flushPromises();
    expect(api.connectCodingAgent).toHaveBeenCalledWith("hermes");
    expect(getText(wrapper, "agent-restart-hint-hermes")).toBe(
      "Start a new Hermes session, or run /reload-mcp in one that is running. A session started with -t loads Hisar when its list names hisar, all or *.",
    );
    wrapper.unmount();
  });

  it("disconnects a connected Codex", async () => {
    api.listAllCodingAgent.mockResolvedValue([{ ...codex, state: "connected" }]);
    const wrapper = await mountSection();
    api.disconnectCodingAgent.mockResolvedValue(codex);
    expect(getText(wrapper, "agent-disconnect-explanation-codex")).toContain("It does not revoke running-agent approvals or saved approvals.");
    await wrapper.find('[data-testid="disconnect-agent-codex"]').trigger("click");
    await flushPromises();
    expect(api.disconnectCodingAgent).toHaveBeenCalledWith("codex");
    expect(hasElement(wrapper, "connect-agent-codex")).toBe(true);
    expect(hasElement(wrapper, "agent-restart-hint-codex")).toBe(false);
    expect(getText(wrapper, "agent-disconnected")).toContain("Disconnected Codex. This removes the connection setup.");
    expect(api.revokeAgentSession).not.toHaveBeenCalled();
    expect(api.revokeAgentGrant).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("names a hisar server it did not add and offers neither Connect nor Disconnect", async () => {
    const detail = "Claude Code already has a server named hisar. Remove it to connect Hisar.";
    api.listAllCodingAgent.mockResolvedValue([{ ...claudeCode, state: "conflict", detail }]);
    const wrapper = await mountSection();
    expect(getText(wrapper, "agent-state-claude-code")).toBe(detail);
    expect(wrapper.find('[data-testid="agent-card-claude-code"] button').exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows why Hisar could not read an agent's settings", async () => {
    api.listAllCodingAgent.mockResolvedValue([{ ...codex, state: "unknown", detail: "Codex could not list its MCP servers: failed to load configuration" }]);
    const wrapper = await mountSection();
    expect(getText(wrapper, "agent-state-codex")).toBe("Codex could not list its MCP servers: failed to load configuration");
    expect(wrapper.find('[data-testid="agent-card-codex"] button').exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows why an agent could not connect and keeps Connect", async () => {
    api.listAllCodingAgent.mockResolvedValue([omp, codex]);
    const wrapper = await mountSection();
    api.connectCodingAgent.mockRejectedValue(new Error("Codex's config.toml changed while Hisar was connecting. Try again"));
    await wrapper.find('[data-testid="connect-agent-codex"]').trigger("click");
    await flushPromises();
    expect(getText(wrapper, "agent-error-codex")).toBe("Codex's config.toml changed while Hisar was connecting. Try again");
    expect(hasElement(wrapper, "agent-error-omp")).toBe(false);
    expect(hasElement(wrapper, "connect-agent-codex")).toBe(true);
    wrapper.unmount();
  });

  it("links to the vault switches while no vault is available to agents", async () => {
    api.listAllCodingAgent.mockResolvedValue([{ ...omp, state: "connected" }, { ...codex, state: "not-installed" }]);
    api.listAllAgentVaultID.mockResolvedValue([]);
    const wrapper = await mountSection();
    expect(getText(wrapper, "agents-no-vaults")).toContain("Coding agents see no secrets until a vault is available to agents.");
    await wrapper.find('[data-testid="show-agent-vaults"]').trigger("click");
    expect(document.activeElement).toBe(wrapper.find('input[name="agentVault-work"]').element);

    api.setVaultAvailableToAgents.mockResolvedValue(["work"]);
    await wrapper.find('input[name="agentVault-work"]').setValue(true);
    await flushPromises();
    expect(hasElement(wrapper, "agents-no-vaults")).toBe(false);
    wrapper.unmount();
  });

  it("asks for no vault while no agent can connect", async () => {
    api.listAllCodingAgent.mockResolvedValue([{ ...omp, state: "not-installed" }]);
    api.listAllAgentVaultID.mockResolvedValue([]);
    const wrapper = await mountSection();
    expect(hasElement(wrapper, "agents-no-vaults")).toBe(false);
    wrapper.unmount();
  });

  it("shows Cursor's settings with get_secret left out", async () => {
    const wrapper = await mountSection();
    expect(JSON.parse(getText(wrapper, "coding-agents-snippet"))).toEqual({ mcpServers: { hisar: { command: "hisar", args: ["mcp", "--no-reveal"] } } });
    expect(wrapper.text()).not.toContain("claude mcp add");
    wrapper.unmount();
  });

  for (const destination of ["agent-sessions", "agent-grants"]) {
    it(`reveals and focuses ${destination} after disconnect under a conflicting search`, async () => {
      api.listAllCodingAgent.mockResolvedValue([{ ...claudeCode, state: "connected" }]);
      api.disconnectCodingAgent.mockResolvedValue(claudeCode);
      const wrapper = mount(SettingsPanel, { attachTo: document.body });
      await flushPromises();
      await wrapper.get('[data-testid="settings-search"]').setValue("disconnect");
      expect(wrapper.get(`#${destination}`).isVisible()).toBe(false);
      await wrapper.get('[data-testid="disconnect-agent-claude-code"]').trigger("click");
      await flushPromises();
      expect(wrapper.get('[data-testid="agent-disconnected"]').text()).toContain("Disconnected Claude Code.");
      await wrapper.get(`[data-testid="agent-disconnected"] a[href="#${destination}"]`).trigger("click");
      expect((wrapper.get('[data-testid="settings-search"]').element as HTMLInputElement).value).toBe("");
      expect(wrapper.get(`#${destination}`).isVisible()).toBe(true);
      expect(document.activeElement).toBe(wrapper.get(`#${destination}`).element);
      wrapper.unmount();
    });
  }
});
