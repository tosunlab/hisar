import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { allTestVault, createApiMock, createCardItem, createItem, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock, createItem: createEmpty } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
    createEmptyItem: () => createEmpty({ id: "", revision: "", title: "", username: "", urls: [], fields: [] }),
    createEmptyCard: () => ({ cardholder: "", type: "", number: "", expiry: "", securityCode: "", pin: "", billingAddress: "", brand: "", last4: "", numberWarning: "", expiryWarning: "" }),
  };
});

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store } = await import("../store");
const MainView = (await import("./MainView.vue")).default;
const ItemDetailPanel = (await import("./ItemDetailPanel.vue")).default;
const ItemEditor = (await import("./ItemEditor.vue")).default;
const ImportPanel = (await import("./ImportPanel.vue")).default;
const api = apiMock.current;

function resetStore(): void {
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
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
  resetStore();
});

describe("Creating a credit card", () => {
  it("opens from the + menu with the card fields and no login fields", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    await wrapper.find('button[aria-label="New item"]').trigger("click");
    await wrapper.find('[data-testid="new-credit-card"]').trigger("click");
    expect(store.editorMode).toBe("create-credit-card");
    for (const name of ["cardholder", "cardNumber", "expiry", "securityCode", "pin"]) {
      expect(wrapper.find(`[data-testid="credit-card-editor"] input[name="${name}"]`).exists()).toBe(true);
    }
    expect(wrapper.find('input[name="cardNumber"]').attributes("type")).toBe("password");
    expect(wrapper.find('input[name="username"]').exists()).toBe(false);
    expect(wrapper.find('input[name="password"]').exists()).toBe(false);
    expect(api.generatePassword).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("shows the brand and warnings from the backend as the number is typed, then saves the card", async () => {
    vi.useFakeTimers();
    store.editorMode = "create-credit-card";
    api.describeCard.mockResolvedValue({ brand: "Visa", last4: "1112", numberWarning: "This doesn't look like a valid card number", expiryWarning: "" });
    const saved = createCardItem();
    api.saveItem.mockResolvedValue(saved);
    api.listAllItem.mockResolvedValue([saved]);
    const wrapper = mount(ItemEditor);
    await wrapper.find('input[name="title"]').setValue("Visa");
    await wrapper.find('input[name="cardNumber"]').setValue("4111 1111 1111 1112");
    await wrapper.find('input[name="securityCode"]').setValue("123");
    await vi.advanceTimersByTimeAsync(300);
    await flushPromises();
    expect(api.describeCard).toHaveBeenCalledWith("4111 1111 1111 1112", "");
    expect((wrapper.find('select[name="cardType"]').element as HTMLSelectElement).value).toBe("Visa");
    expect(wrapper.find('[data-testid="card-number-warning"]').text()).toBe("This doesn't look like a valid card number");
    expect((wrapper.find("button.primary").element as HTMLButtonElement).disabled).toBe(false);

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith(
      "personal",
      expect.objectContaining({ category: "credit-card", title: "Visa", card: expect.objectContaining({ type: "Visa", number: "4111 1111 1111 1112", securityCode: "123" }) }),
    );
    expect(store.editorMode).toBe("view");
  });

  it("formats the number and expiry as they are typed", async () => {
    store.editorMode = "create-credit-card";
    const wrapper = mount(ItemEditor);
    const number = wrapper.find('input[name="cardNumber"]');
    await number.setValue("4444555555555555");
    expect((number.element as HTMLInputElement).value).toBe("4444 5555 5555 5555");
    const expiry = wrapper.find('input[name="expiry"]');
    await expiry.setValue("0130");
    expect((expiry.element as HTMLInputElement).value).toBe("01/30");

    api.saveItem.mockResolvedValue(createCardItem());
    await wrapper.find('input[name="title"]').setValue("Visa");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.saveItem).toHaveBeenCalledWith("personal", expect.objectContaining({ card: expect.objectContaining({ number: "4444 5555 5555 5555", expiry: "01/30" }) }));
  });

  it("lets the user choose a type the number does not point to, with a hint", async () => {
    vi.useFakeTimers();
    store.selectedItem = createCardItem();
    store.editorMode = "edit";
    api.describeCard.mockResolvedValue({ brand: "Mastercard", last4: "4444", numberWarning: "", expiryWarning: "" });
    const wrapper = mount(ItemEditor);
    const cardType = wrapper.find('select[name="cardType"]');
    expect(cardType.findAll("option").map((option) => option.text())).toContain("American Express");

    await wrapper.find('input[name="cardNumber"]').setValue("5555555555554444");
    await vi.advanceTimersByTimeAsync(300);
    await flushPromises();
    expect((cardType.element as HTMLSelectElement).value).toBe("Mastercard");
    expect(wrapper.find('[data-testid="card-type-warning"]').exists()).toBe(false);

    await cardType.setValue("Troy");
    expect(wrapper.find('[data-testid="card-type-warning"]').text()).toBe("The number looks like a Mastercard card");
    await wrapper.find('input[name="cardNumber"]').setValue("5555555555554445");
    await vi.advanceTimersByTimeAsync(300);
    await flushPromises();
    expect((cardType.element as HTMLSelectElement).value).toBe("Troy");
  });

  it("edits a card without sharing its object with the item being viewed", async () => {
    store.selectedItem = createCardItem();
    store.editorMode = "edit";
    const wrapper = mount(ItemEditor);
    expect((wrapper.find('input[name="cardholder"]').element as HTMLInputElement).value).toBe("Alice Smith");
    await wrapper.find('input[name="cardholder"]').setValue("Alice Jones");
    expect(store.selectedItem?.card?.cardholder).toBe("Alice Smith");
  });
});

describe("Viewing a credit card", () => {
  it("shows the brand and last four digits and masks the secrets until revealed", async () => {
    store.selectedItem = createCardItem();
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="card-summary"]').text()).toBe("Visa •••• 1111");
    expect(wrapper.find('[data-testid="card-Card number"]').text()).toBe("•••• •••• •••• 1111");
    expect(wrapper.find('[data-testid="card-Security code"]').text()).not.toContain("123");
    expect(wrapper.find('[data-testid="card-Cardholder"]').text()).toBe("Alice Smith");
    expect(wrapper.find('[data-testid="card-Type"]').text()).toBe("Visa");
    expect(wrapper.text()).toContain("Garanti BBVA");
    await wrapper.find('button[aria-label="Reveal Card number"]').trigger("click");
    expect(wrapper.find('[data-testid="card-Card number"]').text()).toBe("4111 1111 1111 1111");
    await wrapper.find('button[aria-label="Copy Security code"]').trigger("click");
    expect(api.copyField).toHaveBeenCalledWith("personal", "card-1", "Security code");
  });

  it("shows an expired card's warning", () => {
    store.selectedItem = createCardItem({ card: { ...createCardItem().card!, expiry: "09/20", expiryWarning: "This card has expired" } });
    const wrapper = mount(ItemDetailPanel, { props: { canEdit: true, vaultName: "Personal", isShared: false } });
    expect(wrapper.find('[data-testid="card-warning-Expiry"]').text()).toBe("This card has expired");
  });

  it("lists cards with their brand and last four digits", () => {
    store.allItem = [createCardItem({ card: { ...createCardItem().card!, number: "", securityCode: "", pin: "", cardholder: "" } }), createItem()];
    const wrapper = mount(MainView);
    expect(wrapper.findAll(".item-subtitle").map((subtitle) => subtitle.text())).toContain("Visa •••• 1111");
    wrapper.unmount();
  });
});

describe("Importing credit cards", () => {
  it("counts them in the preview", () => {
    store.importPreview = { fileName: "export.json", filePath: "/tmp/export.json", format: "bitwarden-json", formatName: "Bitwarden JSON", vaultId: "personal", vaultName: "Personal", itemCount: 2, loginCount: 1, secureNoteCount: 0, sshKeyCount: 0, creditCardCount: 1, allSkipped: [], allDuplicate: [] };
    store.importStep = "preview";
    const wrapper = mount(ImportPanel);
    const count = wrapper.find('[data-testid="import-count-credit-card"]');
    expect(count.text()).toContain("1");
    expect(count.text()).toContain("Credit card");
  });
});
