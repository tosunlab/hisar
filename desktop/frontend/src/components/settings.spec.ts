import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createAgentHost, createAgentSession, createApiMock, createDevice, createPerson, createReceivedShareInvitation, createSSHAgentStatus, createStatus, createUpdateStatus } from "../testing";
import MainView from "./MainView.vue";
import UnlockView from "./UnlockView.vue";

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
    selectedVaultID: "",
    listVaultID: "",
    isSharingOpen: false,
    isCreatingVault: false,
    allPerson: [],
    allDevice: [],
    shareInvitations: { received: [], sent: [], completionError: "" },
  });
  store.agentErrors = { vaults: "", signIn: "", grants: "", sessions: "", hosts: "", alwaysWhileLocked: "" };
  api.listAllAgentGrant.mockResolvedValue([]);
  api.listAllAgentSession.mockResolvedValue([]);
  api.listAllAgentHost.mockResolvedValue([]);
  api.getSSHAgentStatus.mockResolvedValue(createSSHAgentStatus());
  api.listAllShareInvitation.mockResolvedValue({ received: [], sent: [], completionError: "" });
  api.listAllPerson.mockResolvedValue([]);
});

afterEach(() => {
  wrapper?.unmount();
  wrapper = null;
  Object.defineProperty(navigator, "platform", { value: "MacIntel", configurable: true });
});

describe("Settings sidebar", () => {
  it("lists the sections in order and opens on General", async () => {
    const settings = await mountSettings();
    expect(readAllSectionLabel(settings)).toEqual(["General", "Security", "SSH & Git", "Developer tools", "Browser", "People", "Organizations", "Devices", "Import & Export", "Account", "Help"]);
    expect(settings.find("#settings-title").text()).toBe("General");
    expect(findPage(settings, "general").isVisible()).toBe(true);
    expect(findPage(settings, "security").isVisible()).toBe(false);
    expect(findPage(settings, "general").text()).toContain("Open Hisar at login");
    expect(findPage(settings, "general").text()).not.toContain("Import a Hisar export or another password manager");
    await settings.find('[data-testid="settings-section-export"]').trigger("click");
    expect(settings.find("#settings-title").text()).toBe("Import & Export");
    const transferPage = findPage(settings, "export");
    expect(transferPage.text()).toContain("Import a Hisar export or another password manager");
    expect(transferPage.findAll("section").map((section) => section.attributes("data-testid"))).toEqual(["import-section", "export-section"]);
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

  it("puts Invite to Hisar in the People header and opens the form there", async () => {
    api.listAllPerson.mockResolvedValue([createPerson()]);
    const settings = await mountSettings();
    expect(settings.find('[data-testid="start-invite"]').exists()).toBe(false);
    await settings.find('[data-testid="settings-section-people"]').trigger("click");
    expect(settings.find('[data-testid="start-invite"]').text()).toBe("Invite to Hisar");
    await settings.find('[data-testid="start-invite"]').trigger("click");
    expect(settings.find('[data-testid="invite-form"]').exists()).toBe(true);
    expect(findPage(settings, "people").text()).toContain("Invite to Hisar");
    expect(settings.find('[data-testid="invite-explanation"]').text()).toBe(
      "Creates a Hisar account: the server emails them a sign-up link. To give them a vault, share it from the vault's Share panel.",
    );
    expect(settings.find('[data-testid="start-invite"]').exists()).toBe(false);
    await settings.find('[data-testid="settings-section-devices"]').trigger("click");
    await settings.find('[data-testid="settings-section-people"]').trigger("click");
    expect(settings.find('[data-testid="invite-form"]').exists()).toBe(false);
  });

  it("generates and copies a password from General without an item draft", async () => {
    api.generatePassword.mockResolvedValueOnce("first-password").mockResolvedValue("another-password");
    store.editorMode = "view";
    store.selectedItem = null;
    const settings = await mountSettings();
    await settings.get('[data-testid="open-password-generator"]').trigger("click");
    await flushPromises();
    const dialog = settings.get('[data-testid="password-generator-dialog"]');
    const headingID = dialog.get('[role="dialog"]').attributes("aria-labelledby");
    expect(dialog.get(`#${headingID}`).text()).toBe("Password generator");
    expect(dialog.get('[data-testid="generated-password"]').text()).toBe("first-password");
    await dialog.get('button[aria-label="Regenerate"]').trigger("click");
    await flushPromises();
    await dialog.findAll("button").find((button) => button.text() === "Copy")!.trigger("click");
    await flushPromises();
    expect(api.copyText).toHaveBeenCalledWith("another-password");
    expect(store.noticeMessage).toBe("Password copied. It clears in 90 seconds.");
    expect(store.editorMode).toBe("view");
    expect(store.selectedItem).toBeNull();
    await dialog.findAll("button").find((button) => button.text() === "Close")!.trigger("click");
    expect(settings.find('[data-testid="password-generator-dialog"]').exists()).toBe(false);
    expect(store.isSettingsOpen).toBe(true);
  });

  it("finds the same generator row through Settings search and closes only its modal on Escape", async () => {
    api.generatePassword.mockResolvedValue("generated-password");
    const settings = await mountSettings();
    await search(settings, "password generator");
    expect(readAllSectionLabel(settings)).toEqual(["General"]);
    expect(settings.get('[data-testid="open-password-generator"]').isVisible()).toBe(true);
    await settings.get('[data-testid="open-password-generator"]').trigger("click");
    await flushPromises();
    await settings.get('[data-testid="password-generator-dialog"] input[name="length"]').trigger("keydown", { key: "Escape" });
    await flushPromises();
    expect(settings.find('[data-testid="password-generator-dialog"]').exists()).toBe(false);
    expect(store.isSettingsOpen).toBe(true);
  });
});

describe("Sign-out disclosure", () => {
  it.each([
    { platform: "MacIntel", device: "this Mac" },
    { platform: "Linux x86_64", device: "this computer" },
  ])("explains local removal in Account on $platform", async ({ platform, device }) => {
    Object.defineProperty(navigator, "platform", { value: platform, configurable: true });
    const settings = await mountSettings();
    await settings.get('[data-testid="settings-section-account"]').trigger("click");
    await settings.get('[data-testid="account-section"] button.danger').trigger("click");
    const confirmation = settings.get('[data-testid="settings-sign-out-confirmation"]');
    expect(confirmation.get("h3").text()).toBe(`Sign out of Hisar on ${device}?`);
    expect(confirmation.get("p").text()).toBe(`Hisar removes ${device}'s saved Secret Key and offline copy of your vaults. To sign in again, you need your master password and the Secret Key from your Emergency Kit or another signed-in device.`);
    expect(settings.get('[data-testid="settings-confirm-sign-out"]').text()).toBe("Sign out");
    expect(api.signOut).not.toHaveBeenCalled();
    await confirmation.get("button.ghost").trigger("click");
    expect(settings.find('[data-testid="settings-sign-out-confirmation"]').exists()).toBe(false);
  });

  it.each([
    { platform: "MacIntel", device: "this Mac" },
    { platform: "Linux x86_64", device: "this computer" },
  ])("explains how to save a kit before signing out while locked on $platform", async ({ platform, device }) => {
    Object.defineProperty(navigator, "platform", { value: platform, configurable: true });
    store.status = createStatus("locked");
    wrapper = mount(UnlockView, { attachTo: document.body });
    await wrapper.get(".sign-out > button").trigger("click");
    const confirmation = wrapper.get(".confirm");
    expect(confirmation.get("strong").text()).toBe(`Sign out of Hisar on ${device}?`);
    expect(confirmation.findAll("p")[1].text()).toBe(`Hisar removes ${device}'s saved Secret Key and offline copy of your vaults. To sign in again, you need your master password and the Secret Key from your Emergency Kit or another signed-in device. If you have not saved an Emergency Kit, cancel, unlock Hisar and save one in Settings, Security.`);
    expect(wrapper.get('[data-testid="confirm-sign-out"]').text()).toBe("Sign out");
    expect(api.signOut).not.toHaveBeenCalled();
  });

  it("opens the existing Emergency Kit form from Account without signing out", async () => {
    const settings = await mountSettings();
    await search(settings, "account");
    await settings.get('[data-testid="settings-section-account"]').trigger("click");
    await settings.get('[data-testid="account-section"] button.danger').trigger("click");
    await settings.get('[data-testid="sign-out-save-emergency-kit"]').trigger("click");
    await flushPromises();
    expect(settings.get("#settings-title").text()).toBe("Security");
    expect(settings.get('[data-testid="settings-search"]').element).toHaveProperty("value", "");
    expect(settings.get('input[name="emergencyKitPassword"]').isVisible()).toBe(true);
    expect(settings.find('[data-testid="settings-sign-out-confirmation"]').exists()).toBe(false);
    expect(api.signOut).not.toHaveBeenCalled();
    expect(store.status.state).toBe("unlocked");
  });
});

describe("People empty state", () => {
  it.each([
    { name: "no people", allPerson: [] },
    { name: "only you", allPerson: [createPerson()] },
  ])("offers the same sharing action with $name", async ({ allPerson }) => {
    api.listAllPerson.mockResolvedValue(allPerson);
    const settings = await mountSettings();
    await settings.get('[data-testid="settings-section-people"]').trigger("click");
    expect(settings.get('[data-testid="people-empty-hint"]').text()).toBe("Nobody else yet. Share a vault to see the people you share with here.");
    expect(settings.get('[data-testid="people-share-vault"]').attributes("disabled")).toBeUndefined();
  });

  it("opens the selected vault's Share panel and closes Settings", async () => {
    store.allVault = [{ id: "team", name: "Team", type: "shared", role: "owner", itemCount: 0 }];
    store.selectedVaultID = "team";
    store.listVaultID = "team";
    window.localStorage.setItem("hisar.settings.selectedSection", "people");
    wrapper = mount(MainView);
    await flushPromises();
    await wrapper.get('[data-testid="people-share-vault"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="settings-search"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="vault-sharing-panel"]').text()).toContain("Share “Team”");
    expect(api.listAllVaultMember).toHaveBeenCalledWith("team");
  });

  it("opens the new-vault form when the selected vault is personal, which is never shared", async () => {
    store.allVault = [{ id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0 }];
    store.selectedVaultID = "personal";
    store.listVaultID = "personal";
    window.localStorage.setItem("hisar.settings.selectedSection", "people");
    wrapper = mount(MainView);
    await flushPromises();
    await wrapper.get('[data-testid="people-share-vault"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="settings-search"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="vault-sharing-panel"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="new-vault-form"]').exists()).toBe(true);
    expect(api.listAllOrganization).toHaveBeenCalled();
  });
});

describe("Settings search", () => {
  it.each(["Hisar export", ".hisar", "1Password", "1pux", "Bitwarden", "LastPass", "Chrome", "Firefox", "Safari", "Edge"])("finds the import action for %s", async (source) => {
    const settings = await mountSettings();
    await search(settings, source);
    await settings.get('[data-testid="settings-section-export"]').trigger("click");
    expect(settings.get('[data-testid="import-section"]').isVisible()).toBe(true);
    expect(settings.get('[data-testid="open-import"]').isVisible()).toBe(true);
  });

  it.each(["omp", "Claude Code", "Codex", "Hermes", "Cursor", "Tern"])("keeps coding-agent connections visible for %s", async (name) => {
    const settings = await mountSettings();
    await search(settings, name);
    await settings.get('[data-testid="settings-section-developer"]').trigger("click");
    expect(settings.get('[data-testid="coding-agents-section"]').isVisible()).toBe(true);
    expect(settings.get('[data-testid="agent-connections"]').isVisible()).toBe(true);
  });

  it("keeps a failed grant revocation visible when its owning row is filtered", async () => {
    const grant = { id: "grant", scope: "harness", harnessName: "omp", name: "Deploy", kind: "run", website: "", expiresAt: "" };
    api.listAllAgentGrant.mockResolvedValue([grant]);
    const settings = await mountSettings();
    await search(settings, "Allowed without asking");
    expect(settings.get('[data-testid="coding-agents-section"]').isVisible()).toBe(false);
    api.revokeAgentGrant.mockRejectedValue(new Error("Could not revoke this approval. Try again."));
    await settings.get('[data-testid="revoke-agent-grant-0"]').trigger("click");
    await flushPromises();
    const error = settings.get('[data-testid="agent-grants-section"] [role="alert"]');
    expect(error.isVisible()).toBe(true);
    expect(error.text()).toBe("Could not revoke this approval. Try again.");
    expect(settings.findAll('[data-testid="agent-grant"]')).toHaveLength(1);
  });

  it("keeps independent action and load failures in their own developer-tool sections", async () => {
    api.listAllAgentHost.mockResolvedValue([createAgentHost()]);
    api.listAllAgentSession.mockResolvedValue([createAgentSession()]);
    api.listAllAgentGrant.mockRejectedValue(new Error("Could not load approvals."));
    const settings = await mountSettings();
    await settings.get('[data-testid="settings-section-developer"]').trigger("click");
    api.forgetAgentHost.mockRejectedValue(new Error("Could not forget Tern."));
    await settings.get('[data-testid="forget-agent-host-Tern"]').trigger("click");
    await flushPromises();
    api.revokeAgentSession.mockRejectedValue(new Error("Could not revoke the session."));
    await settings.get('[data-testid="revoke-agent-session-1226"]').trigger("click");
    await flushPromises();
    expect(settings.get('[data-testid="agent-grants-error"]').text()).toBe("Could not load approvals.");
    expect(settings.get('[data-testid="agent-hosts-error"]').text()).toBe("Could not forget Tern.");
    expect(settings.get('[data-testid="agent-sessions-error"]').text()).toBe("Could not revoke the session.");
    api.revokeAgentSession.mockResolvedValue([]);
    await settings.get('[data-testid="revoke-agent-session-1226"]').trigger("click");
    await flushPromises();
    expect(settings.find('[data-testid="agent-sessions-error"]').exists()).toBe(false);
    expect(settings.get('[data-testid="agent-grants-error"]').text()).toBe("Could not load approvals.");
    expect(settings.get('[data-testid="agent-hosts-error"]').text()).toBe("Could not forget Tern.");
    await search(settings, "Agent hosts");
    expect(settings.get('[data-testid="agent-hosts-error"]').isVisible()).toBe(true);
  });

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

  it("finds the read-only clipboard policy separately from auto-lock", async () => {
    const settings = await mountSettings();
    await search(settings, "clipboard");
    expect(readAllSectionLabel(settings)).toEqual(["Security"]);
    expect(settings.get('[data-testid="clipboard-clearing"]').isVisible()).toBe(true);
    expect(settings.get('[data-testid="auto-lock"]').isVisible()).toBe(false);
  });

  it("matches keywords and section names", async () => {
    const settings = await mountSettings();
    await search(settings, "mcp");
    expect(readAllSectionLabel(settings)).toEqual(["Developer tools"]);
    expect(findPage(settings, "developer").find('[data-testid="coding-agents-section"]').isVisible()).toBe(true);
    expect(findPage(settings, "developer").find('[data-testid="command-line-section"]').isVisible()).toBe(false);

    await search(settings, "ssh");
    expect(readAllSectionLabel(settings)).toEqual(["SSH & Git"]);
    expect(findPage(settings, "ssh").text()).toContain("Approve requests on my smart device");
  });

  it("finds the launchers row by searching raycast", async () => {
    const settings = await mountSettings();
    await search(settings, "raycast");
    expect(readAllSectionLabel(settings)).toEqual(["Developer tools"]);
    const developer = findPage(settings, "developer");
    expect(developer.find('[data-testid="command-line-launchers"]').isVisible()).toBe(true);
    expect(developer.find('[data-testid="launcher-apps-empty"]').isVisible()).toBe(true);
    expect(developer.find('input[name="commandLineIntegration"]').isVisible()).toBe(false);
    expect(developer.find('[data-testid="coding-agents-section"]').isVisible()).toBe(false);
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
    expect(findPage(settings, "export").find('[data-testid="import-section"]').isVisible()).toBe(false);
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
    expect(readAllSectionLabel(settings)).toHaveLength(11);
    expect(settings.find('[data-testid="settings-no-results"]').exists()).toBe(false);
    expect(findPage(settings, "general").isVisible()).toBe(true);
    await settings.find('[data-testid="settings-section-export"]').trigger("click");
    expect(findPage(settings, "export").find('[data-testid="import-section"]').isVisible()).toBe(true);
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

describe("Settings help", () => {
  beforeEach(() => {
    store.status = { ...createStatus("unlocked"), server: "https://eu.tryhisar.com" };
    store.updateStatus = null;
    store.allDevice = [];
    api.getUpdateStatus.mockResolvedValue(createUpdateStatus());
    api.listAllDevice.mockResolvedValue([createDevice({ id: "device-mac", isCurrent: true }), createDevice({ id: "device-phone", name: "iPhone", isCurrent: false })]);
    api.copyText.mockResolvedValue(undefined);
  });

  it("shows the support address as text with a Copy button", async () => {
    const settings = await mountSettings();
    await settings.find('[data-testid="settings-section-help"]').trigger("click");
    const help = findPage(settings, "help");
    expect(help.isVisible()).toBe(true);
    expect(help.find('[data-testid="support-email"]').text()).toBe("support@tryhisar.com");
    expect(help.text()).not.toContain("Documentation");
    await help.find('[data-testid="copy-support-email"]').trigger("click");
    await flushPromises();
    expect(api.copyText).toHaveBeenCalledWith("support@tryhisar.com");
  });

  it("shows the versions and copies diagnostics without secrets or other people's details", async () => {
    store.allPerson = [createPerson({ email: "bob@example.com" })];
    const settings = await mountSettings();
    await settings.find('[data-testid="settings-section-help"]').trigger("click");
    const help = findPage(settings, "help");
    expect(help.find('[data-testid="help-app-version"]').text()).toBe("0.2.1 (2001)");
    expect(help.find('[data-testid="help-operating-system"]').text()).toBe("macOS 26.0.1");
    expect(help.find('[data-testid="help-server"]').text()).toBe("https://eu.tryhisar.com");
    expect(help.text()).not.toContain("The server does not report its version.");
    expect(help.find('[data-testid="help-device-id"]').text()).toBe("device-mac");
    await help.find('[data-testid="copy-diagnostics"]').trigger("click");
    await flushPromises();
    const diagnostics = api.copyText.mock.calls[0][0] as string;
    expect(diagnostics).toBe(["Hisar for Mac 0.2.1 (2001)", "System: macOS 26.0.1", "Server: https://eu.tryhisar.com", "Device ID: device-mac"].join("\n"));
    expect(diagnostics).not.toMatch(/alice@example.com|bob@example.com|device-phone/);
  });

  it("is found by searching for support", async () => {
    const settings = await mountSettings();
    await search(settings, "support");
    expect(readAllSectionLabel(settings)).toEqual(["Help"]);
  });
});
