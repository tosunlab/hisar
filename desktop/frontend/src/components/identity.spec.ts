import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createIdentityItem, createItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [], fields: [] }),
    createEmptyIdentity: () => ({
      firstName: "",
      middleName: "",
      lastName: "",
      birthDate: "",
      email: "",
      phone: "",
      address: "",
      passportNumber: "",
      driversLicenseNumber: "",
      nationalIdNumber: "",
    }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store } = await import("../store");
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
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
});

describe("Creating an identity", () => {
  it("opens from the + menu with the identity fields and no login fields", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-identity"]').trigger("click");
    expect(store.editorMode).toBe("create-identity");
    const editor = wrapper.find('[data-testid="identity-editor"]');
    for (const name of ["firstName", "middleName", "lastName", "birthDate", "email", "phone", "passportNumber", "driversLicenseNumber", "nationalIdNumber"]) {
      expect(editor.find(`input[name="${name}"]`).exists()).toBe(true);
    }
    expect(editor.find('input[name="birthDate"]').attributes("type")).toBe("date");
    expect(editor.find('input[name="passportNumber"]').attributes("type")).toBe("password");
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
    expect(wrapper.find('input[name="password"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("saves the identity's values", async () => {
    store.editorMode = "create-identity";
    const saved = createIdentityItem();
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Alice");
    await wrapper.find('input[name="firstName"]').setValue("Alice");
    await wrapper.find('input[name="lastName"]').setValue("Example");
    await wrapper.find('input[name="birthDate"]').setValue("1990-04-12");
    await wrapper.find('input[name="passportNumber"]').setValue("U1234567");
    await wrapper.find("button.ghost.reveal-card").trigger("click");
    expect(wrapper.find('input[name="passportNumber"]').attributes("type")).toBe("text");

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({
        category: "identity",
        title: "Alice",
        identity: expect.objectContaining({ firstName: "Alice", lastName: "Example", birthDate: "1990-04-12", passportNumber: "U1234567" }),
      }),
    );
    expect(store.editorMode).toBe("view");
  });

  it.each(["04/12/1990", "2023-02-31"])("keeps an imported birth date %s that is no calendar day editable as text", (date) => {
    store.selectedItem = createIdentityItem({ identity: { ...createIdentityItem().identity!, birthDate: date } });
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    const birthDate = wrapper.find('input[name="birthDate"]');
    expect(birthDate.attributes("type")).toBe("text");
    expect((birthDate.element as HTMLInputElement).value).toBe(date);
  });

  it("saves an identity whose email is not one address", async () => {
    store.selectedItem = createIdentityItem({ identity: { ...createIdentityItem().identity!, email: "Alice / work" } });
    store.editorMode = "edit";
    api.saveItem.mockResolvedValue(createIdentityItem());
    api.listAllItem.mockResolvedValue([]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="lastName"]').setValue("Sample");
    expect((wrapper.find("form").element as HTMLFormElement).checkValidity()).toBe(true);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({ identity: expect.objectContaining({ email: "Alice / work", lastName: "Sample" }) }),
    );
  });
});

describe("Viewing an identity", () => {
  it("shows the name and masks the document numbers until revealed", async () => {
    store.selectedItem = createIdentityItem();
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="identity-summary"]').text()).toBe("Alice Example");
    expect(wrapper.find('[data-testid="card-First name"]').text()).toBe("Alice");
    expect(wrapper.find('[data-testid="card-Birth date"]').text()).toBe("1990-04-12");
    expect(wrapper.find('[data-testid="card-Middle name"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="card-Passport number"]').text()).not.toContain("U1234567");
    expect(wrapper.text()).toContain("Hisar");
    await wrapper.find('button[aria-label="Reveal Passport number"]').trigger("click");
    expect(wrapper.find('[data-testid="card-Passport number"]').text()).toBe("U1234567");
    await wrapper.find('button[aria-label="Copy Passport number"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "identity-1", "Passport number");
  });

  it("heads an identity with no name with its email, as the list does", () => {
    store.selectedItem = createIdentityItem({ identity: { ...createIdentityItem().identity!, firstName: "", lastName: "" } });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="identity-summary"]').text()).toBe("alice@example.com");
  });

  it("offers a share link for an identity with only identity values", () => {
    store.selectedItem = createIdentityItem({ notes: "", fields: [] });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: false, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="share-link"]').exists()).toBe(true);
  });

  it("lists identities with their name, or the email without one", () => {
    const summary = { ...createIdentityItem().identity!, phone: "", address: "", passportNumber: "", birthDate: "" };
    store.allItem = [
      createIdentityItem({ identity: summary }),
      createIdentityItem({ id: "identity-2", title: "Work", identity: { ...summary, firstName: "", lastName: "" } }),
      createItem(),
    ];
    const wrapper = mount(MainView);
    const allSubtitle = wrapper.findAll(".item-subtitle").map((subtitle) => subtitle.text());
    expect(allSubtitle).toContain("Alice Example");
    expect(allSubtitle).toContain("alice@example.com");
    wrapper.unmount();
  });
});

describe("Importing identities", () => {
  it("counts them in the preview", () => {
    store.importPreview = {
      fileName: "export.json",
      filePath: "/tmp/export.json",
      format: "bitwarden-json",
      formatName: "Bitwarden JSON",
      vaultId: "personal",
      vaultName: "Personal",
      itemCount: 3,
      loginCount: 1,
      secureNoteCount: 0,
      sshKeyCount: 0,
      creditCardCount: 0,
      identityCount: 2,
      apiCredentialCount: 0,
      allSkipped: [],
      allDuplicate: [],
    };
    store.importStep = "preview";
    const wrapper = mount(ImportPanel);
    const count = wrapper.find('[data-testid="import-count-identity"]');
    expect(count.text()).toContain("2");
    expect(count.text()).toContain("Identities");
  });
});
