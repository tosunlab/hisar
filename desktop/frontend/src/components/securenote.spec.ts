import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createSecureNoteItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [] }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const api = apiMock.current;

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    listVaultID: "personal",
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
}

function mountDetail(canEdit = true) {
  return mount(ItemDetailPanel, { props: { canEdit, vaultName: "Personal", vaultType: "personal" } });
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
});

describe("Creating a secure note", () => {
  it("opens from the + menu with only a title, notes and fields", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-secure-note"]').trigger("click");
    expect(store.editorMode).toBe("create-secure-note");
    expect(wrapper.find('[data-testid="secure-note-editor"] textarea[name="notes"]').exists()).toBe(true);
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
    expect(wrapper.find('input[name="password"]').exists()).toBe(false);
    expect(wrapper.find('textarea[name="urls"]').exists()).toBe(false);
    expect(wrapper.find('input[name="oneTimePassword"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("Other fields");
    expect(api.generatePassword).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("saves the note with its category", async () => {
    store.editorMode = "create-secure-note";
    const saved = createSecureNoteItem({ title: "Door codes", notes: "Front 1234" });
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Door codes");
    await wrapper.find('textarea[name="notes"]').setValue("Front 1234");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ category: "secure-note", title: "Door codes", notes: "Front 1234" }));
    expect(store.editorMode).toBe("view");
    expect(store.selectedItem?.category).toBe("secure-note");
  });

  it("edits a note in the notes-first editor", () => {
    store.selectedItem = createSecureNoteItem();
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    expect((wrapper.find('textarea[name="notes"]').element as HTMLTextAreaElement).value).toBe("1. 4821-9930\n2. 1177-0042");
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
    expect(wrapper.find("button.primary").text()).toMatch(/^Save /);
  });
});

describe("Viewing a secure note", () => {
  it("shows the notes first with Copy and no login fields", async () => {
    store.selectedItem = createSecureNoteItem({ fields: [{ name: "Garage", value: "5678", concealed: true }] });
    const wrapper = mountDetail();
    expect(wrapper.find("header").text()).toContain("Secure note");
    expect(wrapper.find('[data-testid="secure-note-notes"] .notes').text()).toBe("1. 4821-9930\n2. 1177-0042");
    expect(wrapper.text()).not.toContain("This item has no fields yet");
    expect(wrapper.find('[data-testid="password-value"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("Garage");
    expect(wrapper.text()).not.toContain("5678");
    await wrapper.find('[data-testid="copy-notes"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "note-1", "notes");
  });

  it("lists notes with their icon and type", () => {
    store.allItem = [createSecureNoteItem({ notes: "" }), createItem()];
    const wrapper = mount(MainView);
    const allSubtitle = wrapper.findAll(".item-subtitle").map((subtitle) => subtitle.text());
    expect(allSubtitle).toContain("Secure note");
    wrapper.unmount();
  });
});

describe("Converting a login to a secure note", () => {
  it("offers it only when the backend says so, and only to editors", () => {
    store.selectedItem = createItem({ username: "", urls: [], notes: "Front 1234", canConvertToSecureNote: true });
    expect(mountDetail().find('[data-testid="convert-to-secure-note"]').exists()).toBe(true);
    expect(mountDetail(false).find('[data-testid="convert-to-secure-note"]').exists()).toBe(false);
    store.selectedItem = createItem({ notes: "work account" });
    expect(mountDetail().find('[data-testid="convert-to-secure-note"]').exists()).toBe(false);
    store.selectedItem = createItem({ username: "", urls: [], fields: [{ name: "PIN", value: "0000", concealed: true }], canConvertToSecureNote: true });
    expect(mountDetail().find('[data-testid="convert-to-secure-note"]').exists()).toBe(true);
  });

  it("converts the item and shows it as a note", async () => {
    const login = createItem({ username: "", urls: [], notes: "Front 1234", canConvertToSecureNote: true });
    store.allItem = [login];
    store.selectedItem = login;
    api.convertToSecureNote.mockResolvedValue({ ...login, category: "secure-note", revision: "rev-2", canConvertToSecureNote: false });
    const wrapper = mountDetail();
    await wrapper.find('[data-testid="convert-to-secure-note"]').trigger("click");
    await flushPromises();
    expect(api.convertToSecureNote).toHaveBeenCalledWith("personal", "item-1");
    expect(store.selectedItem?.category).toBe("secure-note");
    expect(store.allItem[0].category).toBe("secure-note");
    expect(store.noticeMessage).toBe("Converted to a secure note");
    expect(wrapper.find('[data-testid="secure-note-notes"]').exists()).toBe(true);
  });
});
