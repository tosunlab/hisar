import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, ref } from "vue";
import { computeAccessibleName } from "dom-accessibility-api";
import type { createApiMock } from "../testing";
import { createStatus } from "../testing";
import { api } from "../api";
import { store } from "../store";
import CredentialField from "./CredentialField.vue";
import SignInView from "./SignInView.vue";
import ChangeMasterPasswordDialog from "./ChangeMasterPasswordDialog.vue";
import ExportSection from "./ExportSection.vue";

vi.mock("../api", async () => {
  const testing = await vi.importActual<{ createApiMock: typeof createApiMock }>("../testing");
  return { api: testing.createApiMock(), getErrorMessage: String };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

beforeEach(() => {
  store.status = createStatus("signed-out");
  store.isBusy = false;
  store.isSettingsOpen = false;
});

const Form = defineComponent({
  components: { CredentialField },
  setup: () => ({ first: ref("long master password"), repeated: ref("long master password") }),
  template: '<form @submit.prevent><CredentialField><input v-model="first" name="first" type="password" autocomplete="current-password" /></CredentialField><CredentialField secret-name="repeated master password"><input v-model="repeated" name="repeated" type="password" /></CredentialField></form>',
});

describe("credential reveal", () => {
  it("reveals fields independently without replacing the input, value, selection or active control", async () => {
    const wrapper = mount(Form, { attachTo: document.body });
    const input = wrapper.get<HTMLInputElement>('input[name="first"]').element;
    const repeated = wrapper.get<HTMLInputElement>('input[name="repeated"]').element;
    expect(input.type).toBe("password");
    expect(repeated.type).toBe("password");
    input.focus();
    input.setSelectionRange(2, 7, "backward");
    await wrapper.get('button[aria-label="Show master password"]').trigger("click");
    expect(wrapper.get('input[name="first"]').element).toBe(input);
    expect(input.type).toBe("text");
    expect(input.value).toBe("long master password");
    expect([input.selectionStart, input.selectionEnd, input.selectionDirection]).toEqual([2, 7, "backward"]);
    expect(document.activeElement).toBe(input);
    expect(input.autocomplete).toBe("current-password");
    expect(repeated.type).toBe("password");
    const toggle = wrapper.get<HTMLButtonElement>('button[aria-label="Hide master password"]');
    toggle.element.focus();
    await toggle.trigger("click");
    expect(document.activeElement).toBe(toggle.element);
    expect(input.type).toBe("password");
    wrapper.unmount();
  });

  it("conceals on submission, navigation, lock and hidden Settings sections", async () => {
    const wrapper = mount(Form, { attachTo: document.body });
    const input = wrapper.get<HTMLInputElement>('input[name="first"]').element;
    for (const trigger of [
      () => wrapper.get("form").trigger("submit"),
      () => { store.isBusy = true; },
      () => { store.isSettingsOpen = true; },
      () => { store.status = createStatus("locked"); },
      () => { wrapper.element.setAttribute("style", "display: none"); },
    ]) {
      await wrapper.get('button[aria-label="Show master password"]').trigger("click");
      expect(input.type).toBe("text");
      await trigger();
      await flushPromises();
      expect(input.type).toBe("password");
    }
    expect(input.value).toBe("long master password");
    wrapper.unmount();
  });

  it("names account, password-change and export toggles, including fallback authorization", async () => {
    const signIn = mount(SignInView);
    expect(computeAccessibleName(signIn.get(".credential-toggle").element)).toBe("Show master password");
    await signIn.findAll('[role="tab"]')[1]!.trigger("click");
    expect(signIn.findAll(".credential-toggle").map((button) => computeAccessibleName(button.element))).toEqual(["Show master password", "Show repeated master password"]);
    signIn.unmount();
    const change = mount(ChangeMasterPasswordDialog);
    expect(change.findAll(".credential-toggle").map((button) => computeAccessibleName(button.element))).toEqual(["Show current master password", "Show new master password", "Show repeated new master password"]);
    change.unmount();
    vi.mocked(api.getExportOptions).mockResolvedValue({ isBiometricAvailable: false, minPassphraseLength: 10 });
    const exportSection = mount(ExportSection);
    await flushPromises();
    expect(exportSection.findAll(".credential-toggle").map((button) => computeAccessibleName(button.element))).toEqual(["Show export passphrase", "Show repeated export passphrase", "Show master password"]);
    exportSection.unmount();
  });
});
