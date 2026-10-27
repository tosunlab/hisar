import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createItem, createServerItem, createStatus } from "../testing";
import { allIconPreset, getDefaultIconPreset, getWebsiteIconURL } from "../util";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [], fields: [] }),
  };
});

const runtime = vi.hoisted(() => ({ listenerByName: new Map<string, (event: { data: unknown }) => unknown>() }));

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => unknown) => {
      runtime.listenerByName.set(name, listener);
      return () => runtime.listenerByName.delete(name);
    },
  },
}));

const { store } = await import("../store");
const { forgetWebsiteIcons, maxWebsiteIconRequests, startWebsiteIconSync, websiteIconState } = await import("../websiteicons");
const ItemTile = (await import("./ItemTile.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const AppearanceSection = (await import("./AppearanceSection.vue")).default;
const api = apiMock.current;

beforeEach(() => {
  vi.clearAllMocks();
  forgetWebsiteIcons();
  websiteIconState.isEnabled = true;
  api.getWebsiteIcon.mockResolvedValue("");
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: allTestVault.map((vault) => ({ ...vault })),
    selectedVaultID: "personal",
    allItem: [],
    selectedItem: null,
    editorMode: "view",
    errorMessage: "",
    isBusy: false,
  });
});

function describeTile(wrapper: ReturnType<typeof mount>): string {
  const tile = wrapper.find(".tile");
  return tile.attributes("data-image") ?? tile.attributes("data-icon") ?? `initials ${tile.text()}`;
}

describe("Item tiles", () => {
  it("draw the chosen image, then the preset, the website icon, the category icon and the initials", async () => {
    api.getWebsiteIcon.mockResolvedValue("V0VCU0lURQ==");
    const cases: [Record<string, unknown>, string][] = [
      [{ itemIcon: { image: "Q0hPU0VO", preset: "bank" }, websiteURL: "https://github.com" }, "chosen"],
      [{ itemIcon: { preset: "bank", color: "green" }, websiteURL: "https://github.com" }, "bank"],
      [{ websiteURL: "https://github.com" }, "website"],
      [{ icon: "server" }, "server"],
      [{}, "initials GH"],
      [{ itemIcon: { preset: "rocket" }, icon: "server" }, "server"],
    ];
    for (const [props, want] of cases) {
      const wrapper = mount(ItemTile, { props: { title: "Git Hub", ...props } });
      await flushPromises();
      expect(describeTile(wrapper), JSON.stringify(props)).toBe(want);
      wrapper.unmount();
    }
    const colored = mount(ItemTile, { props: { title: "Bank", itemIcon: { preset: "bank", color: "green" } } });
    expect(colored.find(".tile").attributes("data-icon-color")).toBe("green");
    const website = mount(ItemTile, { props: { title: "GitHub", websiteURL: "https://github.com/login" } });
    await flushPromises();
    expect(website.find("img").attributes("src")).toBe("data:image/png;base64,V0VCU0lURQ==");
    expect(api.getWebsiteIcon).toHaveBeenCalledTimes(1);
    colored.unmount();
    website.unmount();
  });

  it("ask for a domain once and at most a few domains at a time", async () => {
    const allResolve: ((icon: string) => void)[] = [];
    api.getWebsiteIcon.mockImplementation(() => new Promise<string>((resolve) => allResolve.push(resolve)));
    const allTile = ["a.example.com", "b.example.com", "c.example.com", "d.example.com", "e.example.com", "f.example.com", "a.example.com"].map((domain) =>
      mount(ItemTile, { props: { title: domain, websiteURL: `https://${domain}/login` } }),
    );
    await flushPromises();
    expect(api.getWebsiteIcon).toHaveBeenCalledTimes(maxWebsiteIconRequests);
    allResolve[0]("");
    await flushPromises();
    expect(api.getWebsiteIcon).toHaveBeenCalledTimes(maxWebsiteIconRequests + 1);
    allResolve.slice(1).forEach((resolve) => resolve(""));
    await flushPromises();
    allResolve.slice(maxWebsiteIconRequests).forEach((resolve) => resolve(""));
    await flushPromises();
    expect(api.getWebsiteIcon).toHaveBeenCalledTimes(6);
    allTile.forEach((tile) => tile.unmount());
  });

  it("show website icons only for logins with an https website and no icon of their own", () => {
    expect(getWebsiteIconURL(createItem({ urls: ["github.com", " https://github.com/login "] }))).toBe("https://github.com/login");
    expect(getWebsiteIconURL(createItem({ urls: ["http://github.com"] }))).toBe("");
    expect(getWebsiteIconURL(createItem({ urls: ["https://github.com"], icon: { preset: "code" } }))).toBe("");
    expect(getWebsiteIconURL(createServerItem({ urls: ["https://admin.example.com"] }))).toBe("");
  });
});

describe("The icon picker", () => {
  it("selects and saves the category preset when only a color is chosen", async () => {
    const wrapper = await openPicker();
    await wrapper.find('[data-testid="icon-color-green"]').trigger("click");
    expect(wrapper.find('[data-testid="item-icon-button"] .tile').attributes("data-icon")).toBe("lock");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    const saved = api.saveItem.mock.calls[0][1];
    expect(saved.icon).toEqual({ preset: "lock", color: "green" });
    wrapper.unmount();
    store.selectedItem = saved;
    store.editorMode = "edit";
    const reopened = mount(ItemEditor);
    expect(reopened.find('[data-testid="item-icon-button"] .tile').attributes("data-icon-color")).toBe("green");
    reopened.unmount();
  });

  it("uses supported presets for every category", () => {
    for (const [category, preset] of Object.entries({
      login: "lock", "credit-card": "card", server: "server", database: "database", "ssh-key": "key",
      "secure-note": "lock", identity: "lock", "api-credential": "lock", address: "lock", "software-license": "lock",
    })) {
      expect(getDefaultIconPreset(category)).toBe(preset);
      expect(allIconPreset).toContain(preset);
    }
  });

  async function openPicker() {
    store.selectedItem = createItem({ id: "bank", title: "Bank", urls: ["https://bank.example.com"] });
    store.editorMode = "edit";
    api.saveItem.mockImplementation(async (_vaultID, detail) => detail);
    const wrapper = mount(ItemEditor, { attachTo: document.body });
    await wrapper.find('[data-testid="item-icon-button"]').trigger("click");
    return wrapper;
  }

  it("saves a preset in a color, and Automatic removes it", async () => {
    const wrapper = await openPicker();
    expect(wrapper.findAll('[data-testid^="icon-preset-"]')).toHaveLength(allIconPreset.length);
    expect(wrapper.find('[data-testid="icon-automatic"]').attributes("aria-pressed")).toBe("true");
    await wrapper.find('[data-testid="icon-color-green"]').trigger("click");
    await wrapper.find('[data-testid="icon-preset-bank"]').trigger("click");
    expect(wrapper.find('[data-testid="item-icon-button"] .tile').attributes("data-icon")).toBe("bank");
    await wrapper.find('[data-testid="icon-color-blue"]').trigger("click");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ icon: { preset: "bank", color: "blue" } }));

    store.editorMode = "edit";
    const second = await openPicker();
    await second.find('[data-testid="icon-preset-heart"]').trigger("click");
    await second.find('[data-testid="icon-automatic"]').trigger("click");
    await second.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenLastCalledWith("personal", expect.objectContaining({ icon: null }));
    wrapper.unmount();
    second.unmount();
  });

  it("uploads an image through the backend and shows why one is refused", async () => {
    const wrapper = await openPicker();
    api.prepareItemIcon.mockResolvedValueOnce("UFJFUEFSRUQ=");
    const input = wrapper.find('[data-testid="icon-file"]');
    Object.defineProperty(input.element, "files", { value: [new File([new Uint8Array([1, 2, 3])], "logo.png", { type: "image/png" })], configurable: true });
    await input.trigger("change");
    await flushPromises();
    await vi.waitFor(() => expect(api.prepareItemIcon).toHaveBeenCalledWith("AQID"));
    await flushPromises();
    expect(wrapper.find('[data-testid="item-icon-button"] .tile').attributes("data-image")).toBe("chosen");
    expect(wrapper.find('[data-testid="icon-color-green"]').attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("Choose a preset icon to color it.");

    api.prepareItemIcon.mockRejectedValueOnce(new Error("choose a PNG, JPEG, GIF or WebP image"));
    await input.trigger("change");
    await vi.waitFor(() => expect(wrapper.find('[data-testid="icon-error"]').exists()).toBe(true));
    expect(wrapper.find('[data-testid="icon-error"]').text()).toBe("choose a PNG, JPEG, GIF or WebP image");

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ icon: { image: "UFJFUEFSRUQ=" } }));
    wrapper.unmount();
  });

  it("closes on Escape and on a click outside", async () => {
    const wrapper = await openPicker();
    await wrapper.find('[data-testid="icon-picker"]').trigger("keydown", { key: "Escape" });
    expect(wrapper.find('[data-testid="icon-picker"]').exists()).toBe(false);
    await wrapper.find('[data-testid="item-icon-button"]').trigger("click");
    document.body.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    await flushPromises();
    expect(wrapper.find('[data-testid="icon-picker"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("Show website icons", () => {
  it("turns website icons off and forgets the ones loaded", async () => {
    api.getWebsiteIcon.mockResolvedValue("V0VCU0lURQ==");
    const tile = mount(ItemTile, { props: { title: "GitHub", websiteURL: "https://github.com" } });
    await flushPromises();
    expect(describeTile(tile)).toBe("website");

    const wrapper = mount(AppearanceSection);
    await flushPromises();
    expect(api.isWebsiteIconsEnabled).toHaveBeenCalled();
    const toggle = wrapper.find('[data-testid="website-icons-switch"]');
    expect((toggle.element as HTMLInputElement).checked).toBe(true);
    await toggle.setValue(false);
    await flushPromises();
    expect(api.setWebsiteIconsEnabled).toHaveBeenCalledWith(false);
    expect(describeTile(tile)).toBe("initials G");

    await toggle.setValue(true);
    await flushPromises();
    expect(describeTile(tile)).toBe("website");
    tile.unmount();
    wrapper.unmount();
  });

  it("follows the setting when another window changes it", async () => {
    api.getWebsiteIcon.mockResolvedValue("V0VCU0lURQ==");
    const stopSync = startWebsiteIconSync();
    const tile = mount(ItemTile, { props: { title: "GitHub", websiteURL: "https://github.com" } });
    await flushPromises();
    expect(describeTile(tile)).toBe("website");

    await runtime.listenerByName.get("website-icons-changed")?.({ data: false });
    await flushPromises();
    expect(describeTile(tile)).toBe("initials G");

    await runtime.listenerByName.get("website-icons-changed")?.({ data: true });
    await flushPromises();
    expect(describeTile(tile)).toBe("website");
    stopSync();
    tile.unmount();
  });
});
