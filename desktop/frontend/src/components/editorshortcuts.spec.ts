import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createStatus } from "../testing";

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
const ItemEditor = (await import("./ItemEditor.vue")).default;
const api = apiMock.current;

function pressKey(target: Element, key: string, modifier: { metaKey?: boolean; ctrlKey?: boolean } = {}): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...modifier });
  target.dispatchEvent(event);
  return event;
}

function mountEditor() {
  return mount(ItemEditor, { attachTo: document.body });
}

beforeEach(() => {
  vi.clearAllMocks();
  document.body.innerHTML = "";
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [],
    selectedItem: createItem({ password: "hunter2hunter2" }),
    editorMode: "edit",
    editorDraft: null,
    discardDraftPrompt: null,
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
  const saved = createItem({ title: "GitHub work" });
  api.saveItem.mockResolvedValue(saved);
  api.listAllItem.mockResolvedValue([saved]);
});

describe("Editor keyboard shortcuts", () => {
  it("leaves an unchanged editor on Esc without asking", async () => {
    const wrapper = mountEditor();
    pressKey(wrapper.find('input[name="username"]').element, "Escape");
    await flushPromises();
    expect(store.editorMode).toBe("view");
    expect(api.saveItem).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("saves on ⌘S from a field and on ⌘↩ from the notes", async () => {
    const wrapper = mountEditor();
    await wrapper.find('input[name="title"]').setValue("GitHub work");
    const event = pressKey(wrapper.find('input[name="title"]').element, "s", { metaKey: true });
    await flushPromises();
    expect(event.defaultPrevented).toBe(true);
    expect(api.saveItem).toHaveBeenCalledTimes(1);
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ title: "GitHub work" }));
    expect(store.editorMode).toBe("view");
    wrapper.unmount();

    store.editorMode = "edit";
    const notesWrapper = mountEditor();
    await notesWrapper.find('textarea[name="notes"]').setValue("Work account");
    pressKey(notesWrapper.find('textarea[name="notes"]').element, "Enter", { ctrlKey: true });
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledTimes(2);
    expect(api.saveItem).toHaveBeenLastCalledWith("personal", expect.objectContaining({ notes: "Work account" }));
    notesWrapper.unmount();
  });

  it("does not save on ⌘S while the title is empty", async () => {
    const wrapper = mountEditor();
    await wrapper.find('input[name="title"]').setValue("");
    pressKey(wrapper.find('input[name="title"]').element, "s", { metaKey: true });
    await flushPromises();
    expect(api.saveItem).not.toHaveBeenCalled();
    expect(store.editorMode).toBe("edit");
    wrapper.unmount();
  });

  it("ignores shortcuts dispatched on Window without throwing or saving", async () => {
    const wrapper = mountEditor();
    const onError = vi.fn();
    window.addEventListener("error", onError);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "s", metaKey: true }));
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(onError).not.toHaveBeenCalled();
    expect(api.saveItem).not.toHaveBeenCalled();
    expect(store.editorMode).toBe("edit");
    window.removeEventListener("error", onError);
    wrapper.unmount();
  });

  it("leaves keys typed outside the editor, such as in Settings, alone", async () => {
    const wrapper = mountEditor();
    const outside = document.createElement("input");
    document.body.append(outside);
    pressKey(outside, "Escape");
    pressKey(outside, "s", { metaKey: true });
    await flushPromises();
    expect(store.editorMode).toBe("edit");
    expect(api.saveItem).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
