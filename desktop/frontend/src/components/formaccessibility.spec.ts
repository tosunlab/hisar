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
  it("associates sign-up mismatch with the repeat field and announces only after blur", async () => {
    const wrapper = mount(SignInView, { attachTo: document.body });
    await wrapper.findAll("button").find((button) => button.text() === "Create account")!.trigger("click");
    await wrapper.find('input[name="password"]').setValue("Long unique master password 92");
    const repeat = wrapper.find('input[name="repeatedPassword"]');
    await repeat.setValue("different");
    expect(repeat.attributes("aria-invalid")).toBe("true");
    expect(computeAccessibleName(repeat.element)).toBe("Repeat master password");
    expect(computeAccessibleDescription(repeat.element)).toBe("The passwords do not match.");
    expect(wrapper.find('[role="status"]').text()).toBe("");
    await repeat.trigger("blur");
    const status = wrapper.find('[role="status"]');
    expect(status.text()).toBe("The passwords do not match.");
    const changed = vi.fn();
    const observer = new MutationObserver(changed);
    observer.observe(status.element, { childList: true, characterData: true, subtree: true });
    await repeat.setValue("still different");
    await repeat.trigger("blur");
    await flushPromises();
    expect(changed).not.toHaveBeenCalled();
    observer.disconnect();
    await repeat.setValue("Long unique master password 92");
    expect(repeat.attributes("aria-invalid")).toBe("false");
    expect(computeAccessibleDescription(repeat.element)).toBe("");
  });

  it("keeps password-change names stable and descriptions separate", async () => {
    const wrapper = mount(ChangeMasterPasswordDialog, { attachTo: document.body });
    const password = wrapper.find('input[name="newPassword"]');
    await password.setValue("short");
    expect(computeAccessibleName(password.element)).toBe("New master password");
    expect(computeAccessibleDescription(password.element)).not.toBe("");
    expect(password.attributes("aria-invalid")).toBe("true");
    await password.trigger("blur");
    expect(wrapper.find('[role="status"]').text()).toBe(wrapper.find('[data-testid="new-password-feedback"]').text());
    await password.setValue("Long unique master password 92");
    const repeat = wrapper.find('input[name="repeatedPassword"]');
    await repeat.setValue("different");
    expect(computeAccessibleName(repeat.element)).toBe("Repeat new master password");
    expect(computeAccessibleDescription(repeat.element)).toBe("The passwords don't match.");
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
