import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import {
  allTestVault,
  createAddressItem,
  createApiMock,
  createCardItem,
  createIdentityItem,
  createImportPreview,
  createItem,
  createStatus,
} from "../testing";

const apiMock = vi.hoisted(() => ({
  current: {} as ReturnType<typeof createApiMock>,
}));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } =
    await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) =>
      error instanceof Error ? error.message : String(error),
    createEmptyItem: () =>
      createEmpty({
        id: "",
        revision: "",
        title: "",
        username: "",
        urls: [],
        fields: [],
      }),
    createEmptyAddress: () => ({
      street: "",
      city: "",
      region: "",
      postalCode: "",
      country: "",
    }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store, filterAllItem } = await import("../store");
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
});

describe("Creating an address", () => {
  it("opens from the + menu with the address fields and no login fields", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-address"]').trigger("click");
    expect(store.editorMode).toBe("create-address");
    const editor = wrapper.find('[data-testid="address-editor"]');
    expect(editor.find('textarea[name="street"]').exists()).toBe(true);
    for (const name of ["city", "region", "postalCode", "country"]) {
      expect(editor.find(`input[name="${name}"]`).exists()).toBe(true);
    }
    expect(wrapper.find('[data-testid="old-address"]').exists()).toBe(false);
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("saves the address's values", async () => {
    store.editorMode = "create-address";
    const saved = createAddressItem();
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Home");
    await wrapper.find('textarea[name="street"]').setValue("1 Main St\nApt 4");
    await wrapper.find('input[name="city"]').setValue("Springfield");
    await wrapper.find('input[name="region"]').setValue("IL");
    await wrapper.find('input[name="postalCode"]').setValue("12345");
    await wrapper.find('input[name="country"]').setValue("US");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({
        category: "address",
        title: "Home",
        address: {
          street: "1 Main St\nApt 4",
          city: "Springfield",
          region: "IL",
          postalCode: "12345",
          country: "US",
        },
      }),
    );
    expect(store.editorMode).toBe("view");
  });
});

describe("Addresses on identities and cards", () => {
  it("edits an identity's address in the address fields and keeps its old one-line address", async () => {
    const identity = createIdentityItem();
    store.selectedItem = createIdentityItem({
      identity: {
        ...identity.identity!,
        street: "",
        city: "",
        region: "",
        postalCode: "",
        country: "",
        address: "1 Main St, Springfield",
      },
    });
    store.editorMode = "edit";
    api.saveItem.mockResolvedValue(store.selectedItem);
    api.listAllItem.mockResolvedValue([]);
    const wrapper = mount(ItemEditor);
    const oldAddress = wrapper.find('[data-testid="old-address"]');
    expect(oldAddress.text()).toContain("Address (old format)");
    expect(
      (oldAddress.find("textarea").element as HTMLTextAreaElement).value,
    ).toBe("1 Main St, Springfield");

    await wrapper.find('input[name="city"]').setValue("Springfield");
    await wrapper.find('[data-testid="use-as-street"]').trigger("click");
    expect(wrapper.find('[data-testid="old-address"]').exists()).toBe(false);
    expect(
      (wrapper.find('textarea[name="street"]').element as HTMLTextAreaElement)
        .value,
    ).toBe("1 Main St, Springfield");

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({
        identity: expect.objectContaining({
          street: "1 Main St, Springfield",
          city: "Springfield",
          address: "",
        }),
      }),
    );
  });

  it("keeps an emptied old address on screen until the item is saved", async () => {
    store.selectedItem = createCardItem({
      card: { ...createCardItem().card!, billingAddress: "1 Main St" },
    });
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    expect(wrapper.find('[data-testid="old-address"]').text()).toContain(
      "Billing address (old format)",
    );
    await wrapper.find('textarea[name="oldAddress"]').setValue("");
    expect(wrapper.find('[data-testid="old-address"]').exists()).toBe(true);
    expect(wrapper.find('input[name="postalCode"]').element).toHaveProperty(
      "value",
      "12345",
    );
  });

  it("shows an identity's address after its phone and a card's billing address in the address fields", () => {
    store.selectedItem = createIdentityItem();
    const identity = mount(ItemDetailPanel, {
      props: { canEdit: true, vaultName: "Personal", vaultType: "personal" },
    });
    const allLabel = identity
      .findAll('[data-testid="identity-fields"] .field-label')
      .map((label) => label.text());
    expect(allLabel.slice(0, 11)).toEqual([
      "First name",
      "Last name",
      "Birth date",
      "Email",
      "Phone",
      "Street",
      "City",
      "Region",
      "Postal code",
      "Country",
      "Passport number",
    ]);
    expect(identity.find('[data-testid="card-Street"]').classes()).toContain(
      "multiline-value",
    );

    store.selectedItem = createCardItem();
    const card = mount(ItemDetailPanel, {
      props: { canEdit: true, vaultName: "Personal", vaultType: "personal" },
    });
    expect(card.find('[data-testid="card-City"]').text()).toBe("Springfield");
    expect(card.find('[data-testid="card-Billing address"]').exists()).toBe(
      false,
    );
  });
});

describe("Viewing an address", () => {
  it("shows its values with copy buttons and the secret reference in the row menu", async () => {
    store.selectedItem = createAddressItem();
    const wrapper = mount(ItemDetailPanel, {
      props: { canEdit: true, vaultName: "Personal", vaultType: "personal" },
    });
    expect(wrapper.find('[data-testid="address-summary"]').text()).toBe(
      "Springfield, US",
    );
    expect(wrapper.find('[data-testid="card-Street"]').text()).toBe(
      "1 Main St\nApt 4",
    );
    expect(wrapper.find('[data-testid="card-Postal code"]').text()).toBe(
      "12345",
    );
    expect(wrapper.text()).toContain("Gate code");
    await wrapper.find('button[aria-label="Copy City"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "home-1", "City");
    await wrapper.find('[data-reference-field="City"] [data-copy-target]').trigger("contextmenu");
    await wrapper.find('[data-testid="context-menu-copy-secret-reference-for-city"]').trigger("click");
    expect(api.copySecretReference).toHaveBeenCalledWith(
      "personal",
      "home-1",
      "City",
    );
  });

  it("offers a share link for an address with only its values", () => {
    store.selectedItem = createAddressItem({ notes: "", fields: [] });
    const wrapper = mount(ItemDetailPanel, {
      props: { canEdit: false, vaultName: "Personal", vaultType: "personal" },
    });
    expect(wrapper.find('[data-testid="share-link"]').exists()).toBe(true);
  });

  it("lists addresses with their city and country and finds them by their street, city or country", () => {
    const allItem = [
      createAddressItem({
        address: {
          street: "1 Main St",
          city: "Springfield",
          region: "",
          postalCode: "",
          country: "US",
        },
      }),
      createAddressItem({
        id: "office-1",
        title: "Office",
        address: {
          street: "Büyükdere Cd. 185",
          city: "",
          region: "",
          postalCode: "",
          country: "",
        },
      }),
      createItem({ username: "carol" }),
    ];
    store.allItem = allItem;
    const wrapper = mount(MainView);
    const allSubtitle = wrapper
      .findAll(".item-subtitle")
      .map((subtitle) => subtitle.text());
    expect(allSubtitle).toContain("Springfield, US");
    expect(allSubtitle).toContain("Büyükdere Cd. 185");
    wrapper.unmount();
    expect(
      filterAllItem(allItem, "springfield").map((vaultItem) => vaultItem.title),
    ).toEqual(["Home"]);
    expect(
      filterAllItem(allItem, "büyükdere").map((vaultItem) => vaultItem.title),
    ).toEqual(["Office"]);
  });
});

describe("Importing addresses", () => {
  it("counts them in the preview", () => {
    store.importPreview = createImportPreview({
      itemCount: 1,
      loginCount: 0,
      addressCount: 1,
      allDuplicate: [],
    });
    store.importStep = "preview";
    const wrapper = mount(ImportPanel);
    const count = wrapper.find('[data-testid="import-count-address"]');
    expect(count.text()).toContain("1");
    expect(count.text()).toContain("address");
  });
});
