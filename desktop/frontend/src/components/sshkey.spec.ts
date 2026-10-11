import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createSSHAgentStatus, createSSHKeyItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const eventListener = vi.hoisted(() => ({ allByName: {} as Record<string, (event: { data: unknown }) => void> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => ({ id: "", revision: "", category: "", title: "", username: "", urls: [], password: "", notes: "", fields: [], publicKey: "", fingerprint: "", keyType: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      eventListener.allByName[name] = listener;
      return () => undefined;
    },
  },
}));

const { store } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const SSHKeyForm = (await import("./SSHKeyForm.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const api = apiMock.current;

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    pendingSecretKey: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
    sshAgentStatus: null,
    sshAgentErrorMessage: "",
    allPendingApproval: [],
  });
}

function findButton(wrapper: ReturnType<typeof mount>, text: string) {
  const button = wrapper.findAll("button").find((candidate) => candidate.text().includes(text));
  if (!button) {
    throw new Error(`no button with text ${text}`);
  }
  return button;
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
});

describe("New item menu", () => {
  it("offers a login and an SSH key", async () => {
    api.generatePassword.mockResolvedValue("generated-password");
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[role="menu"]').exists()).toBe(false);
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    const allMenuItem = wrapper.findAll('[role="menuitem"]');
    expect(allMenuItem.map((menuItem) => menuItem.text())).toEqual([expect.stringContaining("Login"), "SSH key", "Import…"]);

    await wrapper.find('[data-testid="new-ssh-key"]').trigger("click");
    expect(store.editorMode).toBe("create-ssh-key");
    expect(wrapper.find('[role="menu"]').exists()).toBe(false);
    expect(wrapper.find('textarea[name="privateKey"]').exists()).toBe(false);
    expect(wrapper.find('input[name="keyType"]').exists()).toBe(true);

    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-login"]').trigger("click");
    expect(store.editorMode).toBe("create");
    expect(wrapper.find('input[name="username"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("closes on Escape", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(wrapper.find('[role="menu"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows SSH keys with a key tile and the key type", () => {
    store.allItem = [createSSHKeyItem({ publicKey: "", fingerprint: "" }), createItem({ id: "login-1" })];
    const wrapper = mount(MainView);
    const allRow = wrapper.findAll("li");
    expect(allRow[0].find('[data-icon="key"]').exists()).toBe(true);
    expect(allRow[0].text()).toContain("Ed25519 SSH key");
    expect(allRow[1].find('[data-icon="key"]').exists()).toBe(false);
    expect(allRow[1].text()).toContain("alice");
  });
});

describe("SSHKeyForm", () => {
  it("generates an Ed25519 key by default and an RSA key on request, then shows it", async () => {
    const created = createSSHKeyItem({ title: "Deploy key", keyType: "ssh-rsa" });
    api.generateSSHKey.mockResolvedValue(created);
    api.listAllItem.mockResolvedValue([created]);
    store.editorMode = "create-ssh-key";
    const wrapper = mount(SSHKeyForm);
    expect(wrapper.text()).toContain("Recommended");
    expect((wrapper.find('input[value="ed25519"]').element as HTMLInputElement).checked).toBe(true);

    await wrapper.find('input[name="title"]').setValue("Deploy key");
    await wrapper.find('input[value="rsa"]').setValue(true);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.generateSSHKey).toHaveBeenCalledWith("personal", "Deploy key", "rsa");
    expect(store.selectedItem).toEqual(created);
    expect(store.editorMode).toBe("view");
    expect(store.allItem).toEqual([created]);
  });

  it("imports a pasted key with an optional passphrase", async () => {
    const created = createSSHKeyItem({ title: "Old laptop" });
    api.importSSHKey.mockRejectedValueOnce(new Error("this key is protected by a passphrase; enter it to import the key")).mockResolvedValue(created);
    api.listAllItem.mockResolvedValue([created]);
    store.editorMode = "create-ssh-key";
    const wrapper = mount(SSHKeyForm);
    const allTab = wrapper.findAll('[role="tab"]');
    expect(allTab.map((tab) => tab.text())).toEqual(["Generate", "Import"]);
    await allTab[1].trigger("click");
    expect(allTab[1].attributes("aria-selected")).toBe("true");
    expect(wrapper.find('input[name="keyType"]').exists()).toBe(false);
    const passphraseInput = wrapper.find('input[name="passphrase"]');
    expect(passphraseInput.attributes("type")).toBe("password");
    expect(passphraseInput.attributes("required")).toBeUndefined();

    const privateKeyText = "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----";
    await wrapper.find('input[name="title"]').setValue("Old laptop");
    await wrapper.find('textarea[name="privateKey"]').setValue(privateKeyText);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.importSSHKey).toHaveBeenLastCalledWith("personal", "Old laptop", privateKeyText, "");
    expect(store.errorMessage).toContain("protected by a passphrase");
    expect(store.editorMode).toBe("create-ssh-key");

    await wrapper.find('input[name="passphrase"]').setValue("secret phrase");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.importSSHKey).toHaveBeenLastCalledWith("personal", "Old laptop", privateKeyText, "secret phrase");
    expect(store.selectedItem).toEqual(created);
    expect(store.editorMode).toBe("view");
    expect((wrapper.find('input[name="passphrase"]').element as HTMLInputElement).value).toBe("");
  });

  it("needs a title and, for import, a key", async () => {
    store.editorMode = "create-ssh-key";
    const wrapper = mount(SSHKeyForm);
    const submitButton = () => wrapper.find('button[type="submit"]');
    expect(submitButton().attributes("disabled")).toBeDefined();
    await wrapper.find('input[name="title"]').setValue("Laptop");
    expect(submitButton().attributes("disabled")).toBeUndefined();
    await wrapper.findAll('[role="tab"]')[1].trigger("click");
    expect(submitButton().text()).toBe("Import key");
    expect(submitButton().attributes("disabled")).toBeDefined();
    await findButton(wrapper, "Cancel").trigger("click");
    expect(store.editorMode).toBe("view");
  });
});

describe("SSH key detail", () => {
  beforeEach(() => {
    store.selectedItem = createSSHKeyItem({ notes: "Used for GitHub" });
  });

  it("shows the key type, fingerprint, public key and notes", () => {
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.text()).toContain("Ed25519");
    expect(wrapper.find('[data-testid="fingerprint"]').text()).toBe("SHA256:hisarTestFingerprint");
    expect(wrapper.find('[data-testid="fingerprint"]').classes()).toContain("monospace");
    expect(wrapper.find('[data-testid="public-key"]').text()).toBe("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHisarTestKey Laptop key");
    expect(wrapper.find('[data-testid="public-key"]').classes()).toContain("monospace");
    expect(wrapper.text()).toContain("Used for GitHub");
    expect(wrapper.text()).toContain("Hisar's SSH agent offers this key while Hisar is unlocked");
    expect(wrapper.text()).not.toContain("PRIVATE KEY");
    expect(wrapper.text()).not.toContain("Private key");
    expect(wrapper.find('button[aria-label="Reveal password"]').exists()).toBe(false);
  });

  it("copies the public key through the backend", async () => {
    api.copyField.mockResolvedValue(undefined);
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    await wrapper.find('[data-testid="copy-public-key"]').trigger("click");
    await flushPromises();
    expect(api.copyField).toHaveBeenCalledWith("personal", "ssh-1", "public key");
    expect(store.noticeMessage).toContain("Public key copied");
  });

  it("edits only the title and notes", async () => {
    api.saveItem.mockResolvedValue({ ...store.selectedItem!, title: "Work key" });
    api.listAllItem.mockResolvedValue([]);
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    expect(wrapper.find('[data-testid="ssh-key-note"]').exists()).toBe(true);
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
    expect(wrapper.find('input[name="password"]').exists()).toBe(false);
    await wrapper.find('input[name="title"]').setValue("Work key");
    await wrapper.find('textarea[name="notes"]').setValue("Rotated");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ id: "ssh-1", category: "ssh-key", title: "Work key", notes: "Rotated" }));
  });
});

describe("Settings SSH agent section", () => {
  it("shows the running agent and adds Hisar to the SSH config on request", async () => {
    api.configureSSHConfig.mockResolvedValue(createSSHAgentStatus({ isSSHConfigured: true, backupPath: "/Users/alice/.ssh/config.hisar-backup-1" }));
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    expect(api.getSSHAgentStatus).toHaveBeenCalled();
    expect(api.configureSSHConfig).not.toHaveBeenCalled();
    const section = wrapper.find('[data-testid="ssh-agent-section"]');
    expect(section.find('[data-testid="ssh-agent-state"]').text()).toContain("Running at /Users/alice/.hisar/agent.sock");
    expect(section.find('[data-testid="ssh-config-snippet"]').text()).toContain('IdentityAgent "~/.hisar/agent.sock"');
    expect(section.text()).toContain("asks for Touch ID");

    await section.find('[data-testid="configure-ssh-config"]').trigger("click");
    await flushPromises();
    expect(api.configureSSHConfig).toHaveBeenCalledTimes(1);
    expect(wrapper.find('[data-testid="remove-ssh-config"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="ssh-config-snippet"]').exists()).toBe(false);
    expect(store.noticeMessage).toContain("backup saved");
  });

  it("removes Hisar from the SSH config", async () => {
    api.getSSHAgentStatus.mockResolvedValueOnce(createSSHAgentStatus({ isSSHConfigured: true }));
    api.removeSSHConfig.mockResolvedValue(createSSHAgentStatus({ isSSHConfigured: false }));
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    await wrapper.find('[data-testid="remove-ssh-config"]').trigger("click");
    await flushPromises();
    expect(api.removeSSHConfig).toHaveBeenCalled();
    expect(wrapper.find('[data-testid="configure-ssh-config"]').exists()).toBe(true);
  });

  it("turns compact SSH approvals on and off", async () => {
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    const compactSwitch = wrapper.find('input[name="compactSSHApproval"]');
    expect((compactSwitch.element as HTMLInputElement).checked).toBe(false);
    expect(wrapper.find('[data-testid="ssh-agent-section"]').text()).toContain("Compact SSH approvals");
    await compactSwitch.setValue(true);
    await flushPromises();
    expect(api.setCompactSSHApproval).toHaveBeenCalledWith(true);
    expect((wrapper.find('input[name="compactSSHApproval"]').element as HTMLInputElement).checked).toBe(true);
    expect(store.noticeMessage).toBe("SSH approvals now show as a compact card");
    api.setCompactSSHApproval.mockRejectedValueOnce(new Error("could not save desktop.json"));
    await wrapper.find('input[name="compactSSHApproval"]').setValue(false);
    await flushPromises();
    expect(wrapper.find('[data-testid="ssh-agent-section"] [role="alert"]').text()).toBe("could not save desktop.json");
  });

  it("shows why the agent is not running and the in-app approval note", async () => {
    api.getSSHAgentStatus.mockResolvedValueOnce(
      createSSHAgentStatus({ isRunning: false, errorMessage: "another agent is listening", isTouchIDApproval: false }),
    );
    api.configureSSHConfig.mockRejectedValue(new Error("permission denied"));
    const wrapper = mount(SettingsPanel);
    await flushPromises();
    expect(wrapper.find('[data-testid="ssh-agent-state"]').text()).toContain("Not running: another agent is listening");
    expect(wrapper.text()).toContain("asks for your approval in Hisar");
    await wrapper.find('[data-testid="configure-ssh-config"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="ssh-agent-section"] [role="alert"]').text()).toBe("permission denied");
  });
});
