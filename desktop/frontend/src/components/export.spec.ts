import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createApiMock, createStatus } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return {
    api: apiMock.current,
    getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
  };
});

const { store } = await import("../store");
const ExportSection = (await import("./ExportSection.vue")).default;
const api = apiMock.current;

async function mountExport(): Promise<VueWrapper> {
  const wrapper = mount(ExportSection);
  await flushPromises();
  return wrapper;
}

function findSubmit(wrapper: VueWrapper) {
  return wrapper.find('[data-testid="export-submit"]');
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { status: createStatus("unlocked"), errorMessage: "", isBusy: false });
  api.getExportOptions.mockResolvedValue({ isBiometricAvailable: true, minPassphraseLength: 10 });
});

describe("Export section", () => {
  it("needs a long enough passphrase, entered twice the same way", async () => {
    const wrapper = await mountExport();
    expect(findSubmit(wrapper).text()).toBe("Export with Touch ID…");
    expect(findSubmit(wrapper).attributes("disabled")).toBeDefined();

    await wrapper.find('[data-testid="export-passphrase"]').setValue("short");
    expect(wrapper.find('[data-testid="export-passphrase-problem"]').text()).toBe("Use at least 10 characters.");
    await wrapper.find('[data-testid="export-passphrase"]').setValue("orange cactus harbor");
    await wrapper.find('[data-testid="export-passphrase-repeat"]').setValue("orange cactus harbour");
    expect(wrapper.find('[data-testid="export-passphrase-problem"]').text()).toBe("The passphrases don't match.");
    expect(findSubmit(wrapper).attributes("disabled")).toBeDefined();
    await wrapper.find('[data-testid="export-passphrase-repeat"]').setValue("orange cactus harbor");
    expect(findSubmit(wrapper).attributes("disabled")).toBeUndefined();
  });

  it("exports with Touch ID and says where the file went", async () => {
    api.exportAllData.mockResolvedValue({ path: "/Users/alice/Documents/Hisar export 2026-10-11.hisar", vaultCount: 3, itemCount: 42 });
    const wrapper = await mountExport();
    await wrapper.find('[data-testid="export-passphrase"]').setValue("orange cactus harbor");
    await wrapper.find('[data-testid="export-passphrase-repeat"]').setValue("orange cactus harbor");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.exportAllData).toHaveBeenCalledWith({ format: "encrypted", passphrase: "orange cactus harbor", password: "" });
    expect(wrapper.find('[data-testid="export-result"]').text()).toBe("Exported 42 items from 3 vaults to /Users/alice/Documents/Hisar export 2026-10-11.hisar.");
    expect((wrapper.find('[data-testid="export-passphrase"]').element as HTMLInputElement).value).toBe("");
  });

  it("warns that plain JSON is not encrypted and sends no passphrase", async () => {
    api.exportAllData.mockResolvedValue({ path: "/tmp/export.json", vaultCount: 1, itemCount: 1 });
    const wrapper = await mountExport();
    expect(wrapper.find('[data-testid="export-plain-warning"]').exists()).toBe(false);
    await wrapper.find('[data-testid="export-format-plain"]').setValue(true);
    expect(wrapper.find('[data-testid="export-plain-warning"]').text()).toContain("Plain JSON is not encrypted.");
    expect(wrapper.find('[data-testid="export-passphrase"]').exists()).toBe(false);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.exportAllData).toHaveBeenCalledWith({ format: "plain", passphrase: "", password: "" });
    expect(wrapper.find('[data-testid="export-result"]').text()).toBe("Exported 1 item from 1 vault to /tmp/export.json.");
  });

  it("asks for the master password without Touch ID and shows nothing when the save is cancelled", async () => {
    api.getExportOptions.mockResolvedValue({ isBiometricAvailable: false, minPassphraseLength: 10 });
    api.exportAllData.mockResolvedValue({ path: "", vaultCount: 2, itemCount: 5 });
    const wrapper = await mountExport();
    await wrapper.find('[data-testid="export-format-plain"]').setValue(true);
    expect(findSubmit(wrapper).text()).toBe("Export…");
    expect(findSubmit(wrapper).attributes("disabled")).toBeDefined();
    await wrapper.find('[data-testid="export-master-password"]').setValue("master password");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(api.exportAllData).toHaveBeenCalledWith({ format: "plain", passphrase: "", password: "master password" });
    expect(wrapper.find('[data-testid="export-result"]').exists()).toBe(false);
    expect((wrapper.find('[data-testid="export-master-password"]').element as HTMLInputElement).value).toBe("");
  });

  it("shows the backend's error", async () => {
    api.exportAllData.mockRejectedValue(new Error("Touch ID was cancelled"));
    const wrapper = await mountExport();
    await wrapper.find('[data-testid="export-format-plain"]').setValue(true);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(store.errorMessage).toBe("Touch ID was cancelled");
    expect(wrapper.find('[data-testid="export-result"]').exists()).toBe(false);
  });
});
