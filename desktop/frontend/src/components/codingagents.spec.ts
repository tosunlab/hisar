import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createAgentHost, createAgentSession, createApiMock, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const eventListener = vi.hoisted(() => ({ allByName: {} as Record<string, (event: { data: unknown }) => void> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", title: "", username: "", urls: [], password: "", notes: "", fields: [] }),
  };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      eventListener.allByName[name] = listener;
      return () => delete eventListener.allByName[name];
    },
  },
}));

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
    allAgentSession: [],
    allAgentHost: [],
    allAgentGrant: [],
    agentSignIn: { isAvailable: false, isEnabled: false, hasAccessibility: false },
    agentErrors: { vaults: "", signIn: "", grants: "", sessions: "", hosts: "" },
  });
  api.listAllAgentVaultID.mockResolvedValue([]);
  api.listAllAgentGrant.mockResolvedValue([]);
  api.getAgentSignIn.mockResolvedValue({ isAvailable: true, isEnabled: false, hasAccessibility: false });
  api.listAllAgentSession.mockResolvedValue([]);
  api.listAllAgentHost.mockResolvedValue([]);
});

describe("CodingAgentsSection", () => {
  it("lists every vault as invisible to agents by default", async () => {
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect(api.listAllAgentVaultID).toHaveBeenCalledTimes(1);
    expect(wrapper.find('[data-testid="coding-agents-explanation"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("Agents pass references to commands after you approve access here. Commands receive real secrets; masking only reduces accidental disclosure.");
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

describe("CodingAgentsSection sessions and hosts", () => {
  it("lists omp in Tern with its project, its remembered secret and Tern under agent hosts", async () => {
    api.listAllAgentSession.mockResolvedValue([createAgentSession({ firstRequestAt: new Date(Date.now() - 5 * 60_000).toISOString() })]);
    api.listAllAgentHost.mockResolvedValue([createAgentHost()]);
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    const session = wrapper.find('[data-testid="agent-session"]');
    expect(session.find('[data-testid="agent-session-title"]').text()).toBe("omp in Tern, hisarapp-core");
    expect(session.find('[data-testid="agent-session-signed"]').attributes("title")).toBe("omp is signed by its developer");
    expect(session.find("img").attributes("src")).toBe("data:image/png;base64,Tern");
    expect(session.find('[data-testid="agent-session-use"]').text()).toBe("First used 5 min ago · 1 remembered secret");
    expect(session.findAll('[data-testid="agent-session-secrets"] li').map((line) => line.text())).toEqual(["Agents › Deploy token › password"]);
    expect(wrapper.find('[data-testid="agent-host"]').text()).toContain("Tern");
    expect(wrapper.find('[data-testid="agent-hosts-explanation"]').text()).toBe("Apps that ran a coding agent ask for every command-line request without a terminal.");
  });

  it("leaves the checked mark out for omp found by its program name and shows a session without remembered secrets", async () => {
    api.listAllAgentSession.mockResolvedValue([createAgentSession({ isSigned: false, hostName: "", hostIconURL: "", allRememberedSecret: [] })]);
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="agent-session-title"]').text()).toBe("omp, hisarapp-core");
    expect(wrapper.find('[data-testid="agent-session-signed"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="agent-session-use"]').text()).toContain("No remembered secrets");
    expect(wrapper.find('[data-testid="agent-hosts-empty"]').text()).toBe("No app has run a coding agent.");
  });

  it("revokes a session and forgets a host", async () => {
    const session = createAgentSession();
    api.listAllAgentSession.mockResolvedValue([session]);
    api.listAllAgentHost.mockResolvedValue([createAgentHost()]);
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();

    api.revokeAgentSession.mockResolvedValue([]);
    await wrapper.find(`[data-testid="revoke-agent-session-${session.pid}"]`).trigger("click");
    await flushPromises();
    expect(api.revokeAgentSession).toHaveBeenCalledWith(1226, session.startTime);
    expect(wrapper.find('[data-testid="agent-session"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="agent-sessions-empty"]').text()).toBe("No coding agent that used Hisar is running.");
    expect(store.noticeMessage).toBe("omp in Tern asks again the next time it uses a secret");

    api.forgetAgentHost.mockResolvedValue([]);
    await wrapper.find('[data-testid="forget-agent-host-Tern"]').trigger("click");
    await flushPromises();
    expect(api.forgetAgentHost).toHaveBeenCalledWith("/Applications/Tern.app", "VTLHGQC72S");
    expect(wrapper.find('[data-testid="agent-host"]').exists()).toBe(false);
    expect(store.noticeMessage).toBe("Tern shares one approval for its command-line requests again");
  });

  it("lists the sessions again when one starts or ends, until it unmounts", async () => {
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="agent-session"]').exists()).toBe(false);

    api.listAllAgentSession.mockResolvedValue([createAgentSession()]);
    eventListener.allByName["agent-sessions-changed"]({ data: "" });
    await flushPromises();
    expect(wrapper.find('[data-testid="agent-session-title"]').text()).toBe("omp in Tern, hisarapp-core");

    api.listAllAgentSession.mockResolvedValue([]);
    eventListener.allByName["agent-sessions-changed"]({ data: "" });
    await flushPromises();
    expect(wrapper.find('[data-testid="agent-session"]').exists()).toBe(false);
    wrapper.unmount();
    expect(eventListener.allByName["agent-sessions-changed"]).toBeUndefined();
  });

  it("shows why revoking failed", async () => {
    api.listAllAgentSession.mockResolvedValue([createAgentSession()]);
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    api.revokeAgentSession.mockRejectedValue(new Error("the Hisar app could not revoke it"));
    await wrapper.find('[data-testid="revoke-agent-session-1226"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="agent-sessions-error"]').text()).toBe("the Hisar app could not revoke it");
    expect(wrapper.find('[data-testid="agent-session"]').exists()).toBe(true);
  });
});

describe("CodingAgentsSection website sign-in", () => {
  it("is off by default, and turning it on without Accessibility offers System Settings", async () => {
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    const toggle = wrapper.find('[data-testid="agent-sign-in-switch"]');
    expect((toggle.element as HTMLInputElement).checked).toBe(false);
    expect(wrapper.find('[data-testid="agent-sign-in-accessibility"]').exists()).toBe(false);

    api.setAgentSignInEnabled.mockResolvedValue({ isAvailable: true, isEnabled: true, hasAccessibility: false });
    await toggle.setValue(true);
    await flushPromises();
    expect(api.setAgentSignInEnabled).toHaveBeenCalledWith(true);
    expect(wrapper.find('[data-testid="agent-sign-in-accessibility"]').text()).toContain("Hisar needs the Accessibility permission to fill browser windows.");
    await wrapper.find('[data-testid="open-accessibility-settings"]').trigger("click");
    expect(api.openURL).toHaveBeenCalledWith("x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility");

    api.getAgentSignIn.mockResolvedValue({ isAvailable: true, isEnabled: true, hasAccessibility: true });
    window.dispatchEvent(new Event("focus"));
    await flushPromises();
    expect(wrapper.find('[data-testid="agent-sign-in-accessibility"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="agent-sign-in-ready"]').text()).toContain("Hisar can fill browser windows.");
    wrapper.unmount();
  });

  it("cannot be turned on outside Hisar for Mac", async () => {
    api.getAgentSignIn.mockResolvedValue({ isAvailable: false, isEnabled: false, hasAccessibility: false });
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect((wrapper.find('[data-testid="agent-sign-in-switch"]').element as HTMLInputElement).disabled).toBe(true);
    expect(wrapper.find('[data-testid="agent-sign-in-note"]').text()).toBe("Only Hisar for Mac signs agents in to websites.");
  });
});

describe("CodingAgentsSection grants", () => {
  const allGrant = [
    {
      id: "harness/hermes/signin/v/tosunlab/https://tosunlab.com",
      scope: "harness",
      harnessName: "Hermes",
      name: "Agents › Tosun Lab",
      kind: "sign-in",
      website: "https://tosunlab.com",
      expiresAt: new Date(Date.now() + 90 * 60_000).toISOString(),
    },
    { id: "harness/omp/use/v/deploy/password", scope: "harness", harnessName: "omp", name: "Agents › Deploy token › password", kind: "run", website: "", expiresAt: "" },
    {
      id: "connection/7/reveal/v/stripe/api key",
      scope: "connection",
      harnessName: "Claude Desktop",
      name: "Agents › Stripe › api key",
      kind: "reveal",
      website: "",
      expiresAt: new Date(Date.now() + 30 * 60_000).toISOString(),
    },
    {
      id: "session/3/use/v/npm/token",
      scope: "session",
      harnessName: "omp",
      name: "Agents › npm › token",
      kind: "run",
      website: "",
      expiresAt: new Date(Date.now() + 30 * 60_000).toISOString(),
    },
  ];

  it("lists what each agent may do without asking, who holds it and its expiry, and revokes one", async () => {
    api.listAllAgentGrant.mockResolvedValue(allGrant);
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect(wrapper.findAll('[data-testid="agent-grant-name"]').map((name) => name.text())).toEqual([
      "Sign in to https://tosunlab.com as Agents › Tosun Lab",
      "Use Agents › Deploy token › password",
      "Read Agents › Stripe › api key",
      "Use Agents › npm › token",
    ]);
    expect(wrapper.findAll('[data-testid="agent-grant-expiry"]').map((expiry) => expiry.text())).toEqual([
      "Hermes · Expires in 2 hours",
      "omp · Until you revoke it",
      "Claude Desktop, while connected · Expires in 30 minutes",
      "omp, this session · Expires in 30 minutes",
    ]);

    api.revokeAgentGrant.mockResolvedValue(allGrant.slice(0, 2).concat(allGrant[3]));
    await wrapper.find('[data-testid="revoke-agent-grant-2"]').trigger("click");
    await flushPromises();
    expect(api.revokeAgentGrant).toHaveBeenCalledWith("connection/7/reveal/v/stripe/api key");
    expect(wrapper.findAll('[data-testid="agent-grant"]')).toHaveLength(3);
    expect(store.noticeMessage).toBe("Claude Desktop asks again the next time it needs Agents › Stripe › api key");
  });

  it("says every agent asks when nothing is allowed without asking", async () => {
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect(wrapper.find('[data-testid="agent-grants-empty"]').text()).toBe("Every agent asks before it uses a secret.");
  });

  it("names a session's sign-in flow by its website", async () => {
    api.listAllAgentSession.mockResolvedValue([
      createAgentSession({ allRememberedSecret: [{ name: "Agents › Tosun Lab", isReveal: false, website: "https://tosunlab.com", expiresAt: new Date(Date.now() + 4 * 60_000).toISOString() }] }),
    ]);
    const wrapper = mount(CodingAgentsSection);
    await flushPromises();
    expect(wrapper.findAll('[data-testid="agent-session-secrets"] li').map((line) => line.text())).toEqual(["Sign in to https://tosunlab.com as Agents › Tosun Lab"]);
  });
});
