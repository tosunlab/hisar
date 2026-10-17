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
const MainView = (await import("./MainView.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const api = apiMock.current;

const gitHub = createItem({ id: "github", title: "GitHub", tags: ["work", "dev"] });
const bank = createItem({ id: "bank", title: "Bank", username: "alice@bank.example", urls: [], tags: ["Finance"] });
const untagged = createItem({ id: "notes", title: "Notes", username: "", urls: [] });

function resetStore(): void {
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [gitHub, bank, untagged],
    selectedItem: null,
    editorMode: "view",
    searchQuery: "",
    selectedTag: "",
    errorMessage: "",
    noticeMessage: "",
    isBusy: false,
    isSettingsOpen: false,
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  resetStore();
});

describe("Tags in the sidebar", () => {
  it("lists every tag with its count and filters the items by the chosen one", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    const tagList = wrapper.find('[data-testid="tag-list"]');
    expect(tagList.findAll("button").map((button) => `${button.find(".vault-name").text()} ${button.find(".vault-count").text()}`)).toEqual(["dev 1", "Finance 1", "work 1"]);
    expect(wrapper.find('[data-testid="tag-filter"]').exists()).toBe(false);

    await wrapper.find('[data-testid="tag-work"]').trigger("click");
    expect(store.selectedTag).toBe("work");
    expect(wrapper.findAll(".item .item-title").map((title) => title.text())).toEqual(["GitHub"]);
    expect(wrapper.find('[data-testid="tag-filter"]').text()).toContain("work");
    expect(wrapper.find('[data-testid="tag-work"]').attributes("aria-pressed")).toBe("true");

    await wrapper.find('[data-testid="tag-filter"] button[aria-label="Show all items"]').trigger("click");
    expect(store.selectedTag).toBe("");
    expect(wrapper.findAll(".item .item-title")).toHaveLength(3);
    wrapper.unmount();
  });

  it("explains an empty list behind a tag and a search", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('[data-testid="tag-Finance"]').trigger("click");
    await wrapper.find('input[name="search"]').setValue("github");
    expect(wrapper.find(".list-empty").text()).toContain("No items match your search.");
    await wrapper.find('input[name="search"]').setValue("");
    expect(wrapper.findAll(".item .item-title").map((title) => title.text())).toEqual(["Bank"]);
    wrapper.unmount();
  });

  it("hides the tag list for a vault without tags", () => {
    store.allItem = [untagged];
    const wrapper = mount(MainView, { attachTo: document.body });
    expect(wrapper.find('[data-testid="tag-list"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("Tags in the item view", () => {
  it("shows the tags as chips that filter the list", async () => {
    store.selectedItem = gitHub;
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    const chips = wrapper.findAll('[data-testid="item-tags"] button');
    expect(chips.map((chip) => chip.text())).toEqual(["work", "dev"]);
    await chips[1].trigger("click");
    expect(store.selectedTag).toBe("dev");
    expect(wrapper.find('[data-testid="item-tags"] button.selected').text()).toBe("dev");
  });

  it("shows no tag row for an item without tags", () => {
    store.selectedItem = untagged;
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", vaultType: "personal" } });
    expect(wrapper.find('[data-testid="item-tags"]').exists()).toBe(false);
  });
});

describe("Tags in the editor", () => {
  it("adds tags on Enter and comma, removes them, and saves them with the item", async () => {
    store.editorMode = "create";
    api.generatePassword.mockResolvedValue("generated-password");
    const saved = createItem({ id: "new", title: "Router", tags: ["home", "network"] });
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Router");
    const tagInput = wrapper.find('input[name="tags"]');
    await tagInput.setValue("home");
    await tagInput.trigger("keydown", { key: "Enter" });
    await tagInput.setValue("network, home, dropped");
    await tagInput.trigger("keydown", { key: "," });
    expect(wrapper.findAll('[data-testid="tag-editor"] .tag-chip').map((chip) => chip.text())).toEqual(["home", "network", "dropped"]);
    expect((tagInput.element as HTMLInputElement).value).toBe("");

    await wrapper.find('button[aria-label="Remove tag dropped"]').trigger("click");
    expect(wrapper.findAll('[data-testid="tag-editor"] .tag-chip').map((chip) => chip.text())).toEqual(["home", "network"]);

    await tagInput.trigger("keydown", { key: "Backspace" });
    expect(wrapper.findAll('[data-testid="tag-editor"] .tag-chip').map((chip) => chip.text())).toEqual(["home"]);

    await tagInput.setValue("network");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ title: "Router", tags: ["home", "network"] }));
    expect(store.selectedItem?.tags).toEqual(["home", "network"]);
  });

  it("starts from the item's tags when editing and keeps them when nothing changes", async () => {
    store.selectedItem = gitHub;
    store.editorMode = "edit";
    api.saveItem.mockResolvedValue(gitHub);
    api.listAllItem.mockResolvedValue([gitHub, bank, untagged]);
    const wrapper = mount(ItemEditor);
    expect(wrapper.findAll('[data-testid="tag-editor"] .tag-chip').map((chip) => chip.text())).toEqual(["work", "dev"]);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ id: "github", tags: ["work", "dev"] }));
    expect(gitHub.tags).toEqual(["work", "dev"]);
  });

  it("offers tags for SSH keys and secure notes too", () => {
    store.editorMode = "create-secure-note";
    const noteEditor = mount(ItemEditor);
    expect(noteEditor.find('[data-testid="tag-editor"]').exists()).toBe(true);
    noteEditor.unmount();
    store.selectedItem = createItem({ id: "key", title: "Deploy key", category: "ssh-key", publicKey: "ssh-ed25519 AAAA", tags: ["servers"] });
    store.editorMode = "edit";
    const keyEditor = mount(ItemEditor);
    expect(keyEditor.findAll('[data-testid="tag-editor"] .tag-chip').map((chip) => chip.text())).toEqual(["servers"]);
  });
});
