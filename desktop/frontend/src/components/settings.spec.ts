import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createApiMock, createPerson, createReceivedShareInvitation, createSSHAgentStatus, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", category: "", title: "", username: "", urls: [], password: "", notes: "", fields: [], publicKey: "", fingerprint: "", keyType: "" }),
  };
});

const { store } = await import("../store");
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const api = apiMock.current;

let wrapper: VueWrapper | null = null;

async function mountSettings(): Promise<VueWrapper> {
  wrapper = mount(SettingsPanel, { attachTo: document.body });
  await flushPromises();
  return wrapper;
}

function readAllSectionLabel(settings: VueWrapper): string[] {
  return settings.findAll(".section-button .section-name").map((label) => label.text());
}

function findPage(settings: VueWrapper, sectionID: string) {
  return settings.find(`[data-testid="settings-page-${sectionID}"]`);
}

function readSelectedSection(settings: VueWrapper): string {
  return settings.find('.section-button[aria-current="page"]').attributes("data-testid") ?? "";
}

async function search(settings: VueWrapper, text: string): Promise<void> {
  await settings.find('[data-testid="settings-search"]').setValue(text);
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  Object.assign(store, {
    status: createStatus("unlocked"),
    errorMessage: "",
    isBusy: false,
    isSettingsOpen: true,
    allVault: [],
    allPerson: [],
    allDevice: [],
    shareInvitations: { received: [], sent: [], completionError: "" },
  });
  api.getSSHAgentStatus.mockResolvedValue(createSSHAgentStatus());
  api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [], completionError: "" });
  api.listAllPerson.mockResolvedValue([]);
});

afterEach(() => {
  wrapper?.unmount();
  wrapper = null;
});

describe("Settings sidebar", () => {
  it("lists the sections in order and opens on General", async () => {
    const settings = await mountSettings();
    expect(readAllSectionLabel(settings)).toEqual(["General", "Security", "SSH & Git", "Developer tools", "Browser", "People", "Devices", "Account"]);
    expect(settings.find("#settings-title").text()).toBe("General");
    expect(findPage(settings, "general").isVisible()).toBe(true);
    expect(findPage(settings, "security").isVisible()).toBe(false);
    expect(findPage(settings, "general").text()).toContain("Open Hisar at login");
    expect(findPage(settings, "general").text()).toContain("Import from another password manager");
  });

  it("shows the chosen section and keeps every section's rows where they belong", async () => {
    const settings = await mountSettings();
    await settings.find('[data-testid="settings-section-ssh"]').trigger("click");
    expect(settings.find("#settings-title").text()).toBe("SSH & Git");
    expect(findPage(settings, "ssh").isVisible()).toBe(true);
    expect(findPage(settings, "general").isVisible()).toBe(false);
    expect(findPage(settings, "ssh").text()).toContain("Compact SSH approvals");
    expect(findPage(settings, "ssh").find('[data-testid="commit-signing"]').text()).toContain("Remember commit signing");
    expect((findPage(settings, "ssh").find('[data-testid="signing-remember"]').element as HTMLSelectElement).value).toBe("0");
    expect(findPage(settings, "developer").find('[data-testid="command-line-section"]').exists()).toBe(true);
    expect(findPage(settings, "developer").find('[data-testid="coding-agents-section"]').exists()).toBe(true);
    expect(findPage(settings, "security").find('[data-testid="enable-touch-id"]').exists()).toBe(true);
    expect(findPage(settings, "browser").find('[data-testid="browser-extension-section"]').exists()).toBe(true);
    expect(findPage(settings, "devices").find('[data-testid="devices-section"]').exists()).toBe(true);
    expect(findPage(settings, "account").text()).toContain("Sign out");
  });

  it("remembers the last section for the next time Settings opens", async () => {
    const settings = await mountSettings();
    await settings.find('[data-testid="settings-section-developer"]').trigger("click");
    settings.unmount();
    wrapper = null;

    const reopened = await mountSettings();
    expect(readSelectedSection(reopened)).toBe("settings-section-developer");
    expect(reopened.find("#settings-title").text()).toBe("Developer tools");
  });

  it("falls back to General when the remembered section is unknown", async () => {
    window.localStorage.setItem("hisar.settings.selectedSection", "advanced");
    const settings = await mountSettings();
    expect(readSelectedSection(settings)).toBe("settings-section-general");
  });

  it("moves between sections with the arrow keys", async () => {
    const settings = await mountSettings();
    const general = settings.find('[data-testid="settings-section-general"]');
    await general.trigger("keydown", { key: "ArrowDown" });
    await flushPromises();
    expect(readSelectedSection(settings)).toBe("settings-section-security");
    expect(document.activeElement?.getAttribute("data-testid")).toBe("settings-section-security");
    await settings.find('[data-testid="settings-section-security"]').trigger("keydown", { key: "ArrowUp" });
    await settings.find('[data-testid="settings-section-general"]').trigger("keydown", { key: "ArrowUp" });
    await flushPromises();
    expect(readSelectedSection(settings)).toBe("settings-section-general");
    expect(settings.find('[data-testid="settings-section-general"]').attributes("tabindex")).toBe("0");
    expect(settings.find('[data-testid="settings-section-security"]').attributes("tabindex")).toBe("-1");
  });

  it("shows the number of invitations for you as a badge on People", async () => {
    api.listAllShareInvitation.mockResolvedValue({ received: [createReceivedShareInvitation(), createReceivedShareInvitation({ id: "second" })], sent: [], completionError: "" });
    const settings = await mountSettings();
    const badge = settings.find('[data-testid="settings-section-people"] [data-testid="people-invitation-badge"]');
    expect(badge.text()).toBe("2");
    expect(badge.attributes("aria-label")).toBe("2 invitations");
  });

  it("shows no badge without invitations for you", async () => {
    const settings = await mountSettings();
    expect(settings.find('[data-testid="people-invitation-badge"]').exists()).toBe(false);
  });

  it("puts Invite in the People header and opens the form there", async () => {
    api.listAllPerson.mockResolvedValue([createPerson()]);
    const settings = await mountSettings();
    expect(settings.find('[data-testid="start-invite"]').exists()).toBe(false);
    await settings.find('[data-testid="settings-section-people"]').trigger("click");
    await settings.find('[data-testid="start-invite"]').trigger("click");
    expect(settings.find('[data-testid="invite-form"]').exists()).toBe(true);
    expect(settings.find('[data-testid="start-invite"]').exists()).toBe(false);
    await settings.find('[data-testid="settings-section-devices"]').trigger("click");
    await settings.find('[data-testid="settings-section-people"]').trigger("click");
    expect(settings.find('[data-testid="invite-form"]').exists()).toBe(false);
  });
});

describe("Settings search", () => {
  it("keeps only the sections and rows that match", async () => {
    const settings = await mountSettings();
    await search(settings, "touch id");
    expect(readAllSectionLabel(settings)).toEqual(["Security"]);
    expect(settings.find("#settings-title").text()).toBe("Security");
    const security = findPage(settings, "security");
    expect(security.find('[data-testid="enable-touch-id"]').isVisible()).toBe(true);
    expect(security.find('[data-testid="auto-lock"]').isVisible()).toBe(false);
    expect(security.find('[data-testid="save-emergency-kit"]').isVisible()).toBe(false);
  });

  it("matches keywords and section names", async () => {
    const settings = await mountSettings();
    await search(settings, "mcp");
    expect(readAllSectionLabel(settings)).toEqual(["Developer tools"]);
    expect(findPage(settings, "developer").find('[data-testid="coding-agents-section"]').isVisible()).toBe(true);
    expect(findPage(settings, "developer").find('[data-testid="command-line-section"]').isVisible()).toBe(false);

    await search(settings, "ssh");
    expect(readAllSectionLabel(settings)).toEqual(["SSH & Git"]);
    expect(findPage(settings, "ssh").text()).toContain("Allow approvals from my iPhone");
  });

  it("finds rows in several sections and moves the selection to the first one", async () => {
    const settings = await mountSettings();
    await settings.find('[data-testid="settings-section-account"]').trigger("click");
    await search(settings, "sign out");
    expect(readAllSectionLabel(settings)).toEqual(["Devices", "Account"]);
    expect(readSelectedSection(settings)).toBe("settings-section-account");

    await search(settings, "update");
    expect(readSelectedSection(settings)).toBe("settings-section-general");
    expect(findPage(settings, "general").find('[data-testid="updates-section"]').isVisible()).toBe(true);
    expect(findPage(settings, "general").find('[data-testid="open-at-login-section"]').isVisible()).toBe(false);
    expect(findPage(settings, "general").find('[data-testid="import-section"]').isVisible()).toBe(false);
  });

  it("leaves out invitation rows that do not exist", async () => {
    const settings = await mountSettings();
    await search(settings, "accept");
    expect(readAllSectionLabel(settings)).toEqual([]);
  });

  it("says when nothing matches and shows everything again after clearing", async () => {
    const settings = await mountSettings();
    await search(settings, "kubernetes");
    expect(readAllSectionLabel(settings)).toEqual([]);
    expect(settings.find('[data-testid="settings-no-results"]').text()).toContain("No settings match “kubernetes”");
    expect(settings.find(".no-section").text()).toBe("No results");
    expect(findPage(settings, "general").isVisible()).toBe(false);

    await search(settings, "");
    expect(readAllSectionLabel(settings)).toHaveLength(8);
    expect(settings.find('[data-testid="settings-no-results"]').exists()).toBe(false);
    expect(findPage(settings, "general").isVisible()).toBe(true);
    expect(findPage(settings, "general").find('[data-testid="import-section"]').isVisible()).toBe(true);
  });

  it("focuses the search with Command-F and clears it with Escape before closing", async () => {
    const settings = await mountSettings();
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "f", metaKey: true }));
    const searchInput = settings.find('[data-testid="settings-search"]');
    expect(document.activeElement).toBe(searchInput.element);

    await search(settings, "browser");
    searchInput.element.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await flushPromises();
    expect((searchInput.element as HTMLInputElement).value).toBe("");
    expect(store.isSettingsOpen).toBe(true);

    searchInput.element.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    expect(store.isSettingsOpen).toBe(false);
  });

  it("moves from the search to the selected section with the down arrow", async () => {
    const settings = await mountSettings();
    await search(settings, "browser");
    await settings.find('[data-testid="settings-search"]').trigger("keydown", { key: "ArrowDown" });
    await flushPromises();
    expect(document.activeElement?.getAttribute("data-testid")).toBe("settings-section-browser");
  });
});
