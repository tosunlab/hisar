import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createStatus } from "../testing";
import { api } from "../api";
import { store } from "../store";
import SettingsPanel from "./SettingsPanel.vue";

vi.mock("../api", async () => {
  const { createApiMock } = await import("../testing");
  return { api: createApiMock(), getErrorMessage: (error: unknown) => String(error) };
});


beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  Object.assign(store, { status: { ...createStatus("unlocked"), biometricAvailable: true }, isBusy: false, errorMessage: "" });
});

for (const form of [
  { name: "Touch ID", button: "enable-touch-id", field: "touchIDPassword", query: "Emergency Kit", operation: "enableBiometricUnlock" },
  { name: "Emergency Kit", button: "save-emergency-kit", field: "emergencyKitPassword", query: "Touch ID", operation: "saveEmergencyKit" },
] as const) {
  describe(`${form.name} password`, () => {
    async function open() {
      const wrapper = mount(SettingsPanel);
      await wrapper.get('[data-testid="settings-section-security"]').trigger("click");
      await wrapper.get(`[data-testid="${form.button}"]`).trigger("click");
      await wrapper.get(`input[name="${form.field}"]`).setValue("synthetic master password");
      return wrapper;
    }

    it("clears on cancel and reopen", async () => {
      const wrapper = await open();
      await wrapper.findAll("button").find((button) => button.text() === "Cancel")!.trigger("click");
      await wrapper.get(`[data-testid="${form.button}"]`).trigger("click");
      expect((wrapper.get(`input[name="${form.field}"]`).element as HTMLInputElement).value).toBe("");
      wrapper.unmount();
    });

    it("clears when leaving Security", async () => {
      const wrapper = await open();
      await wrapper.get('[data-testid="settings-section-general"]').trigger("click");
      await wrapper.get('[data-testid="settings-section-security"]').trigger("click");
      expect(wrapper.find(`input[name="${form.field}"]`).exists()).toBe(false);
      await wrapper.get(`[data-testid="${form.button}"]`).trigger("click");
      expect((wrapper.get(`input[name="${form.field}"]`).element as HTMLInputElement).value).toBe("");
      wrapper.unmount();
    });

    it("clears when search hides the row", async () => {
      const wrapper = await open();
      await wrapper.get('[data-testid="settings-search"]').setValue(form.query);
      await wrapper.get('[data-testid="settings-search"]').setValue("");
      await wrapper.get('[data-testid="settings-section-security"]').trigger("click");
      expect(wrapper.find(`input[name="${form.field}"]`).exists()).toBe(false);
      await wrapper.get(`[data-testid="${form.button}"]`).trigger("click");
      expect((wrapper.get(`input[name="${form.field}"]`).element as HTMLInputElement).value).toBe("");
      wrapper.unmount();
    });

    it("clears after submission", async () => {
      vi.mocked(api.saveEmergencyKit).mockResolvedValue("/tmp/kit.pdf");
      vi.mocked(api.enableBiometricUnlock).mockResolvedValue({ ...store.status, biometricEnabled: false });
      const wrapper = await open();
      const input = wrapper.get(`input[name="${form.field}"]`).element as HTMLInputElement;
      input.form!.dispatchEvent(new Event("submit"));
      await flushPromises();
      expect(api[form.operation]).toHaveBeenCalledWith("synthetic master password");
      await wrapper.get(`[data-testid="${form.button}"]`).trigger("click");
      expect((wrapper.get(`input[name="${form.field}"]`).element as HTMLInputElement).value).toBe("");
      wrapper.unmount();
    });
  });
}
