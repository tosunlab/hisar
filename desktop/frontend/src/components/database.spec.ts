import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createDatabaseItem, createImportPreview, createItem, createStatus } from "../testing";
import { formatDatabaseSummary } from "../util";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [], fields: [] }),
    createEmptyDatabase: () => ({ engine: "postgresql", engineName: "PostgreSQL", host: "", port: "", name: "", options: "", connectionString: "", portWarning: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store, filterAllItem, hasShareableField } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const ImportPanel = (await import("./ImportPanel.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [createItem({ id: "login-1", title: "GitHub" })],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
});

describe("Creating a database", () => {
  it("opens from the + menu with the database's fields and the username and password, but no websites", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-database"]').trigger("click");
    expect(store.editorMode).toBe("create-database");
    const editor = wrapper.find('[data-testid="database-editor"]');
    for (const name of ["host", "port", "databaseName", "options"]) {
      expect(editor.find(`input[name="${name}"]`).exists()).toBe(true);
    }
    const allEngine = editor.findAll('select[name="engine"] option').map((option) => option.text());
    expect(allEngine).toEqual(["PostgreSQL", "MySQL", "MariaDB", "SQL Server", "Oracle", "MongoDB", "Redis", "Other"]);
    expect((editor.find('select[name="engine"]').element as HTMLSelectElement).value).toBe("postgresql");
    expect(editor.find('input[name="otherEngine"]').exists()).toBe(false);
    expect(wrapper.find('input[name="username"]').exists()).toBe(true);
    expect(wrapper.find('input[name="password"]').attributes("type")).toBe("password");
    expect(wrapper.find('textarea[name="urls"]').exists()).toBe(false);
    expect(wrapper.find('input[name="oneTimePassword"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("saves the database's values with its login data", async () => {
    store.editorMode = "create-database";
    const saved = createDatabaseItem();
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Prod db");
    await wrapper.find('select[name="engine"]').setValue("mysql");
    await wrapper.find('input[name="host"]').setValue("db.example.com");
    await wrapper.find('input[name="port"]').setValue("70000");
    expect(wrapper.find('[data-testid="database-port-warning"]').text()).toBe("The port is not a number from 1 to 65535");
    await wrapper.find('input[name="port"]').setValue("3306");
    expect(wrapper.find('[data-testid="database-port-warning"]').exists()).toBe(false);
    await wrapper.find('input[name="databaseName"]').setValue("shop");
    await wrapper.find('input[name="options"]').setValue("tls=true");
    await wrapper.find('input[name="username"]').setValue("root");
    await wrapper.find('input[name="password"]').setValue("db-password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({
        category: "database",
        title: "Prod db",
        username: "root",
        password: "db-password",
        urls: [],
        database: expect.objectContaining({ engine: "mysql", host: "db.example.com", port: "3306", name: "shop", options: "tls=true" }),
      }),
    );
    expect(store.editorMode).toBe("view");
  });

  it("keeps an engine Hisar does not know as Other with its text", async () => {
    store.selectedItem = createDatabaseItem({ database: { ...createDatabaseItem().database!, engine: "DB2", engineName: "DB2", connectionString: "" } });
    store.editorMode = "edit";
    api.saveItem.mockResolvedValue(store.selectedItem);
    const wrapper = mount(ItemEditor);
    expect((wrapper.find('select[name="engine"]').element as HTMLSelectElement).value).toBe("other");
    expect((wrapper.find('input[name="otherEngine"]').element as HTMLInputElement).value).toBe("DB2");
    expect(wrapper.text()).toContain("Hisar builds connection strings only for the engines listed.");
    await wrapper.find('select[name="engine"]').setValue("redis");
    expect(wrapper.find('input[name="otherEngine"]').exists()).toBe(false);
    await wrapper.find('select[name="engine"]').setValue("other");
    expect((wrapper.find('input[name="otherEngine"]').element as HTMLInputElement).value).toBe("");
    await wrapper.find('input[name="otherEngine"]').setValue("Cassandra");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ database: expect.objectContaining({ engine: "Cassandra" }) }));
  });
});

describe("Viewing a database", () => {
  it("lists its values and masks the connection string until revealed", async () => {
    store.selectedItem = createDatabaseItem();
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="database-summary"]').text()).toBe("PostgreSQL · db.example.com:5432/app");
    expect(wrapper.find('[data-testid="database-Engine"]').text()).toBe("PostgreSQL");
    expect(wrapper.find('[data-testid="database-Host"]').text()).toBe("db.example.com");
    expect(wrapper.find('[data-testid="database-Port"]').text()).toBe("5432");
    expect(wrapper.find('[data-testid="database-Database"]').text()).toBe("app");
    expect(wrapper.find('[data-testid="database-options"]').text()).toBe("sslmode=require");
    const connectionString = () => wrapper.find('[data-testid="connection-string-value"]').text();
    expect(connectionString()).not.toContain("db-password");
    await wrapper.find('button[aria-label="Reveal connection string"]').trigger("click");
    expect(connectionString()).toBe("postgresql://app:db-password@db.example.com:5432/app?sslmode=require");
    await wrapper.find('button[aria-label="Copy connection string"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "db-1", "connection string");
    await wrapper.find('button[aria-label="Copy secret reference for connection string"]').trigger("click");
    expect(api.copySecretReference).toHaveBeenCalledWith("personal", "db-1", "connection string");
    await wrapper.find('button[aria-label="Copy secret reference for Host"]').trigger("click");
    expect(api.copySecretReference).toHaveBeenCalledWith("personal", "db-1", "Host");
    expect(wrapper.text()).not.toContain("Website");
  });

  it("shows no connection string for an engine Hisar does not know", () => {
    store.selectedItem = createDatabaseItem({ database: { ...createDatabaseItem().database!, engine: "DB2", engineName: "DB2", connectionString: "" } });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="database-connection-string"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="database-Engine"]').text()).toBe("DB2");
  });

  it("warns under a port that is not a number from 1 to 65535", () => {
    store.selectedItem = createDatabaseItem({ database: { ...createDatabaseItem().database!, port: "70000", portWarning: "The port is not a number from 1 to 65535" } });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="database-port-warning"]').text()).toBe("The port is not a number from 1 to 65535");
  });

  it("lists databases with their engine, address and name and finds them by any of them", () => {
    const summary = { ...createDatabaseItem().database!, options: "", connectionString: "" };
    const allItem = [
      createDatabaseItem({ title: "Prod", database: summary }),
      createDatabaseItem({ id: "cache-1", title: "Cache", username: "", database: { ...summary, engine: "redis", engineName: "Redis", host: "2001:db8::1", port: "6379", name: "" } }),
      createItem({ username: "carol" }),
    ];
    store.allItem = allItem;
    const wrapper = mount(MainView);
    const allSubtitle = wrapper.findAll(".item-subtitle").map((subtitle) => subtitle.text());
    expect(allSubtitle).toContain("PostgreSQL · db.example.com:5432/app");
    expect(allSubtitle).toContain("Redis · [2001:db8::1]:6379");
    wrapper.unmount();
    const findTitles = (query: string) => filterAllItem(allItem, query).map((vaultItem) => vaultItem.title);
    expect(findTitles("postgres")).toEqual(["Prod"]);
    expect(findTitles("db.example.com:5432/app")).toEqual(["Prod"]);
    expect(findTitles("5432")).toEqual(["Prod"]);
    expect(findTitles("[2001:db8::1]:6379")).toEqual(["Cache"]);
    expect(findTitles("db-password")).toEqual([]);
    expect(formatDatabaseSummary(null)).toBe("Database");
  });

  it("offers a share link for a database with only its values", () => {
    expect(hasShareableField(createDatabaseItem({ username: "", password: "", fields: [], notes: "" }))).toBe(true);
  });
});

describe("Importing databases", () => {
  it("counts them in the preview", () => {
    store.importPreview = createImportPreview({ itemCount: 2, loginCount: 1, databaseCount: 1 });
    store.importStep = "preview";
    const wrapper = mount(ImportPanel);
    const count = wrapper.find('[data-testid="import-count-database"]');
    expect(count.text()).toContain("1");
    expect(count.text()).toContain("Database");
  });
});
