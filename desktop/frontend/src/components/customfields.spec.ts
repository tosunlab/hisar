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

const gitHub = createItem({ id: "github", title: "GitHub", fields: [{ name: "Recovery code", value: "recovery-code-1", concealed: true }] });

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [gitHub],
    selectedItem: null,
    editorMode: "view",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
  });
});

describe("Other fields in the editor", () => {
  it("asks for a name before saving a value without one and keeps the draft", async () => {
    store.editorMode = "create";
    api.generatePassword.mockResolvedValue("generated-password");
    const saved = createItem({ id: "new", title: "GitHub", fields: [{ name: "Recovery code", value: "recovery-code-1", concealed: false }] });
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor, { attachTo: document.body });
    await wrapper.find('input[name="title"]').setValue("GitHub");
    await wrapper.find("button.add-field").trigger("click");
    await wrapper.find("button.add-field").trigger("click");
    await wrapper.find('input[name="fieldName0"]').setValue("  ");
    await wrapper.find('input[name="fieldValue0"]').setValue("recovery-code-1");

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).not.toHaveBeenCalled();
    expect(wrapper.findAll('[data-testid="field-name-error"]').map((error) => error.text())).toEqual(["Enter a name for this field."]);
    expect(document.activeElement).toBe(wrapper.find('input[name="fieldName0"]').element);
    expect(wrapper.find('input[name="fieldName0"]').attributes("aria-invalid")).toBe("true");
    expect((wrapper.find('input[name="fieldValue0"]').element as HTMLInputElement).value).toBe("recovery-code-1");
    expect((wrapper.find('input[name="title"]').element as HTMLInputElement).value).toBe("GitHub");

    await wrapper.find('input[name="fieldName0"]').setValue("Recovery code");
    expect(wrapper.find('[data-testid="field-name-error"]').exists()).toBe(false);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({
        title: "GitHub",
        fields: [
          { name: "Recovery code", value: "recovery-code-1", concealed: false },
          { name: "", value: "", concealed: false },
        ],
      }),
    );
    wrapper.unmount();
  });

  it("refuses to save a stored field whose name was cleared", async () => {
    store.selectedItem = gitHub;
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor, { attachTo: document.body });
    await wrapper.find('input[name="fieldName0"]').setValue("");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).not.toHaveBeenCalled();
    expect(wrapper.find('[data-testid="field-name-error"]').text()).toBe("Enter a name for this field.");
    expect(document.activeElement).toBe(wrapper.find('input[name="fieldName0"]').element);
    expect((wrapper.find('input[name="fieldValue0"]').element as HTMLInputElement).value).toBe("recovery-code-1");
    wrapper.unmount();
  });
});
