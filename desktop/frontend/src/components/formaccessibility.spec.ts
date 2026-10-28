import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { computeAccessibleDescription, computeAccessibleName } from "dom-accessibility-api";
import { createStatus } from "../testing";
import { store } from "../store";
import SignInView from "./SignInView.vue";
import ChangeMasterPasswordDialog from "./ChangeMasterPasswordDialog.vue";
import ExportSection from "./ExportSection.vue";

// Vitest hoists mock factories before the static fixture imports initialize.
vi.mock("../api", async () => {
  const { createApiMock, createItem } = await import("../testing");
  return { api: createApiMock(), getErrorMessage: String, createEmptyItem: createItem };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));
enableAutoUnmount(afterEach);
beforeEach(() => { Object.assign(store, { status: createStatus("signed-out"), errorMessage: "", isBusy: false }); });

describe("form correction accessibility", () => {
  it("names missing fields and focuses the first correction on keyboard submit", async () => {
    const wrapper = mount(SignInView, { attachTo: document.body });
    await wrapper.findAll("button").find((button) => button.text() === "Create account")!.trigger("click");
    await flushPromises();
    await wrapper.get("form").trigger("submit");
    const name = wrapper.get('input[name="name"]');
    expect(document.activeElement).toBe(name.element);
    expect(computeAccessibleName(name.element)).toBe("Name, required");
    expect(computeAccessibleDescription(name.element)).toBe("Enter your name.");
    await name.setValue("Ada");
    await wrapper.get('input[name="password"]').setValue("Long unique master password 92");
    const repeat = wrapper.get('input[name="repeatedPassword"]');
    await repeat.setValue("different");
    await wrapper.get("form").trigger("submit");
    expect(repeat.attributes("aria-invalid")).toBe("true");
    expect(computeAccessibleName(repeat.element)).toBe("Repeat master password, required");
    expect(computeAccessibleDescription(repeat.element)).toBe("The master passwords don't match.");
    await repeat.setValue("Long unique master password 92");
    expect(repeat.attributes("aria-invalid")).toBe("false");
  });

  it("keeps password-change names stable and descriptions separate", async () => {
    const wrapper = mount(ChangeMasterPasswordDialog, { attachTo: document.body });
    const password = wrapper.find('input[name="newPassword"]');
    await password.setValue("hunter2");
    await flushPromises();
    expect(computeAccessibleName(password.element)).toBe("New master password");
    expect(computeAccessibleDescription(password.element)).not.toBe("");
    expect(password.attributes("aria-invalid")).toBe("true");
    await password.trigger("blur");
    expect(wrapper.find('[role="status"]').text()).toBe(wrapper.find('[data-testid="new-password-feedback"]').text());
    await password.setValue("Long unique master password 92");
    await flushPromises();
    const repeat = wrapper.find('input[name="repeatedPassword"]');
    await repeat.setValue("different");
    expect(computeAccessibleName(repeat.element)).toBe("Repeat new master password");
    expect(computeAccessibleDescription(repeat.element)).toBe("The passwords don't match.");
  });

  it("associates the same-password correction with the new password", async () => {
    const wrapper = mount(ChangeMasterPasswordDialog, { attachTo: document.body });
    const password = wrapper.find('input[name="newPassword"]');
    await wrapper.find('input[name="currentPassword"]').setValue("Long unique master password 92");
    await password.setValue("Long unique master password 92");
    await flushPromises();
    expect(password.attributes("aria-invalid")).toBe("true");
    expect(computeAccessibleDescription(password.element)).toContain("The new master password is the same as the current one.");
    await password.trigger("blur");
    expect(wrapper.find('[role="status"]').text()).toBe("The new master password is the same as the current one.");
  });

  it("associates export requirements and mismatches with the field to correct", async () => {
    const wrapper = mount(ExportSection, { attachTo: document.body });
    await flushPromises();
    const passphrase = wrapper.find('[data-testid="export-passphrase"]');
    const repeat = wrapper.find('[data-testid="export-passphrase-repeat"]');
    await passphrase.setValue("short");
    expect(passphrase.attributes("aria-invalid")).toBe("true");
    expect(computeAccessibleDescription(passphrase.element)).toContain("Use at least");
    await passphrase.trigger("blur");
    expect(wrapper.find('[role="status"]').text()).toContain("Use at least");
    await passphrase.setValue("long export passphrase");
    await repeat.setValue("different passphrase");
    expect(passphrase.attributes("aria-invalid")).toBe("false");
    expect(repeat.attributes("aria-invalid")).toBe("true");
    expect(computeAccessibleDescription(repeat.element)).toBe("The passphrases don't match.");
    await repeat.trigger("blur");
    expect(wrapper.find('[role="status"]').text()).toBe("The passphrases don't match.");
  });
});
