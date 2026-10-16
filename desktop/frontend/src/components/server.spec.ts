import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createImportPreview, createItem, createServerItem, createSSHKeyItem, createStatus } from "../testing";
import { formatServerSummary } from "../util";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [], fields: [] }),
    createEmptyServer: () => ({ host: "", port: "", sshKeyId: "", sshKeyTitle: "", sshKeyFingerprint: "", portWarning: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store, filterAllItem, hasShareableField } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const ImportPanel = (await import("./ImportPanel.vue")).default;
const api = apiMock.current;

const deployKey = createSSHKeyItem({ id: "key-1", title: "Deploy key" });
const laptopKey = createSSHKeyItem({ id: "key-2", title: "Laptop key" });

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [deployKey, laptopKey, createItem({ id: "login-1", title: "GitHub" })],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
});

describe("Creating a server", () => {
  it("opens from the + menu with the server's fields, the login fields and the vault's SSH keys", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-server"]').trigger("click");
    expect(store.editorMode).toBe("create-server");
    const editor = wrapper.find('[data-testid="server-editor"]');
    expect(editor.find('input[name="host"]').exists()).toBe(true);
    expect(editor.find('input[name="port"]').exists()).toBe(true);
    expect(editor.findAll('select[name="sshKey"] option').map((option) => option.text())).toEqual(["None", "Deploy key", "Laptop key"]);
    expect(wrapper.find('input[name="username"]').exists()).toBe(true);
    expect(wrapper.find('input[name="password"]').attributes("type")).toBe("password");
    expect(wrapper.text()).toContain("Admin console URL");
    wrapper.unmount();
  });

  it("saves the server's values with its login data and linked key, and warns about a port out of range", async () => {
    store.editorMode = "create-server";
    const saved = createServerItem();
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Build server");
    await wrapper.find('input[name="host"]').setValue("build.example.com");
    await wrapper.find('input[name="port"]').setValue("70000");
    expect(wrapper.find('[data-testid="server-port-warning"]').text()).toBe("The port is not a number from 1 to 65535");
    await wrapper.find('input[name="port"]').setValue("2222");
    expect(wrapper.find('[data-testid="server-port-warning"]').exists()).toBe(false);
    await wrapper.find('select[name="sshKey"]').setValue("key-1");
    await wrapper.find('input[name="username"]').setValue("deploy");
    await wrapper.find('input[name="password"]').setValue("server-password");
    await wrapper.find('textarea[name="urls"]').setValue("https://console.example.com");

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({
        category: "server",
        username: "deploy",
        password: "server-password",
        urls: ["https://console.example.com"],
        server: expect.objectContaining({ host: "build.example.com", port: "2222", sshKeyId: "key-1" }),
      }),
    );
    expect(store.editorMode).toBe("view");
  });

  it("keeps a link to a deleted key chosen as missing until another is picked", async () => {
    store.selectedItem = createServerItem({ server: { ...createServerItem().server!, sshKeyId: "gone", sshKeyTitle: "" } });
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    const select = wrapper.find('select[name="sshKey"]');
    expect((select.element as HTMLSelectElement).value).toBe("gone");
    expect(select.findAll("option").map((option) => option.text())).toEqual(["None", "Missing SSH key", "Deploy key", "Laptop key"]);
    await select.setValue("");
    expect(select.findAll("option").map((option) => option.text())).toEqual(["None", "Deploy key", "Laptop key"]);
  });
});

describe("Viewing a server", () => {
  it("shows the host, port, login data, admin console and the linked key, which opens", async () => {
    store.selectedItem = createServerItem();
    api.getItem.mockResolvedValue(deployKey);
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="server-summary"]').text()).toBe("build.example.com:2222 · deploy");
    expect(wrapper.find('[data-testid="server-host"]').text()).toBe("build.example.com:2222");
    expect(wrapper.find('[data-testid="server-port"]').text()).toBe("2222");
    expect(wrapper.find('[data-testid="password-value"]').text()).not.toContain("server-password");
    expect(wrapper.text()).toContain("Admin console");
    expect(wrapper.find('[data-testid="server-ssh-key"]').text()).toContain("Deploy key");
    expect(wrapper.find('[data-testid="server-ssh-key"]').text()).toContain("SHA256:abc");

    await wrapper.find('button[aria-label="Copy Host"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "build-1", "Host");
    await wrapper.find('button[aria-label="Open website"]').trigger("click");
    expect(api.openURL).toHaveBeenCalledWith("https://console.example.com");
    await wrapper.find('button[aria-label="Open SSH key"]').trigger("click");
    await flushPromises();
    expect(api.getItem).toHaveBeenCalledWith("personal", "key-1");
    expect(store.selectedItem?.id).toBe("key-1");
  });

  it("says when the linked key is gone", () => {
    store.selectedItem = createServerItem({ server: { ...createServerItem().server!, sshKeyTitle: "", sshKeyFingerprint: "" } });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="server-ssh-key"]').text()).toContain("Missing SSH key");
    expect(wrapper.find('button[aria-label="Open SSH key"]').exists()).toBe(false);
  });

  it("lists servers with their host and username and finds them by host", () => {
    const allItem = [
      createServerItem({ title: "Build", server: { ...createServerItem().server!, sshKeyId: "", sshKeyTitle: "" } }),
      createServerItem({ id: "db-1", title: "Database", username: "", server: { ...createServerItem().server!, host: "2001:db8::1", port: "5432" } }),
      createItem({ username: "carol" }),
    ];
    store.allItem = allItem;
    const wrapper = mount(MainView);
    const allSubtitle = wrapper.findAll(".item-subtitle").map((subtitle) => subtitle.text());
    expect(allSubtitle).toContain("build.example.com:2222 · deploy");
    expect(allSubtitle).toContain("[2001:db8::1]:5432");
    wrapper.unmount();
    expect(filterAllItem(allItem, "build.example").map((vaultItem) => vaultItem.title)).toEqual(["Build"]);
    expect(filterAllItem(allItem, "2001:db8").map((vaultItem) => vaultItem.title)).toEqual(["Database"]);
    expect(formatServerSummary(null, "")).toBe("Server");
  });

  it("offers a share link for a server with only its host", () => {
    expect(hasShareableField(createServerItem({ username: "", password: "", urls: [], fields: [], notes: "" }))).toBe(true);
  });
});

describe("Importing servers", () => {
  it("counts them in the preview", () => {
    store.importPreview = createImportPreview({ itemCount: 2, loginCount: 1, serverCount: 1 });
    store.importStep = "preview";
    const wrapper = mount(ImportPanel);
    const count = wrapper.find('[data-testid="import-count-server"]');
    expect(count.text()).toContain("1");
    expect(count.text()).toContain("Server");
  });
});
