import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createItem, createStatus, type ApiMock } from "../testing";
import type { VaultSummary } from "../api";
import { store } from "../store";
import App from "../App.vue";

const apiMock = vi.hoisted(() => ({ current: {} as ApiMock }));

// vi.mock runs before this file's imports, so its factory loads the fixtures itself.
vi.mock("../api", async () => {
  const { createApiMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createApiMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [] }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const api = apiMock.current;

enableAutoUnmount(afterEach);

const allVault: VaultSummary[] = [{ id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 1 }];
const github = createItem({ id: "github", title: "GitHub", password: "github-password" });

async function mountApp() {
  const wrapper = mount(App, { attachTo: document.body });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  document.body.innerHTML = "";
  Object.assign(store, {
    status: createStatus("signed-out"),
    allVault: [],
    selectedVaultID: "",
    allItem: [],
    selectedItem: null,
    isFavoritesSelected: false,
    editorMode: "view",
    searchQuery: "",
    selectedTag: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
  api.getStatus.mockResolvedValue(createStatus("unlocked"));
  api.listAllVault.mockResolvedValue(allVault);
  api.listAllItem.mockResolvedValue([github]);
  api.getItem.mockResolvedValue(github);
  api.listAllFavoriteItem.mockResolvedValue([]);
});

describe("Where errors show", () => {
  it("shows a failed copy from the item list as an error notice, not in the detail column", async () => {
    api.copyField.mockRejectedValue(new Error("the clipboard is not available"));
    const wrapper = await mountApp();
    await wrapper.find(".list .item").trigger("contextmenu", { clientX: 40, clientY: 60 });
    await flushPromises();
    await wrapper.find('[data-testid="context-menu-copy-password"]').trigger("click");
    await flushPromises();
    const notice = wrapper.find('[data-testid="notice"]');
    expect(notice.text()).toBe("the clipboard is not available");
    expect(notice.classes()).toContain("notice-error");
    expect(notice.attributes("role")).toBe("alert");
    expect(wrapper.find(".detail-error").exists()).toBe(false);
  });

  it("keeps a copy that worked a plain notice", async () => {
    api.copyField.mockResolvedValue(undefined);
    const wrapper = await mountApp();
    await wrapper.find(".list .item").trigger("contextmenu", { clientX: 40, clientY: 60 });
    await flushPromises();
    await wrapper.find('[data-testid="context-menu-copy-password"]').trigger("click");
    await flushPromises();
    const notice = wrapper.find('[data-testid="notice"]');
    expect(notice.classes()).not.toContain("notice-error");
    expect(notice.attributes("role")).toBe("status");
  });

  it("shows why a new vault failed in its form, and clears it when the form opens again", async () => {
    api.createVault.mockRejectedValue(new Error("A vault named Personal already exists"));
    const wrapper = await mountApp();
    await wrapper.find('[data-testid="new-vault"]').trigger("click");
    await flushPromises();
    await wrapper.find('input[name="vaultName"]').setValue("Personal");
    await wrapper.find('[data-testid="new-vault-form"]').trigger("submit");
    await flushPromises();
    expect(wrapper.find('[data-testid="new-vault-error"]').text()).toBe("A vault named Personal already exists");
    expect(wrapper.find(".detail-error").exists()).toBe(false);

    await wrapper.findAll('[data-testid="new-vault-form"] button').find((button) => button.text() === "Cancel")!.trigger("click");
    await wrapper.find('[data-testid="new-vault"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-testid="new-vault-error"]').exists()).toBe(false);
  });

  it("closes the detail column's error", async () => {
    const wrapper = await mountApp();
    store.errorMessage = "cannot reach the Hisar server; check your connection";
    await flushPromises();
    expect(wrapper.find(".detail-error").text()).toBe("cannot reach the Hisar server; check your connection");
    await wrapper.find('[data-testid="dismiss-error"]').trigger("click");
    expect(store.errorMessage).toBe("");
    expect(wrapper.find(".detail-error").exists()).toBe(false);
  });
});
