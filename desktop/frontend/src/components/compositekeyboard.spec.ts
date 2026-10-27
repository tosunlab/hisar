import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { applyTheme } from "../theme";
import AppearanceSection from "./AppearanceSection.vue";
import SignInView from "./SignInView.vue";
import SSHKeyForm from "./SSHKeyForm.vue";
import type { createApiMock } from "../testing";

vi.mock("../api", async () => {
  const testing = await vi.importActual<{ createApiMock: typeof createApiMock }>("../testing");
  return { api: testing.createApiMock(), getErrorMessage: String };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

beforeEach(() => applyTheme({ appearance: "system", color: "neutral" }));

describe("segmented keyboard controls", () => {
  it("offers one radio Tab stop and selects and focuses with arrows, including wrapping", async () => {
    const wrapper = mount(AppearanceSection, { attachTo: document.body });
    for (const group of wrapper.findAll('[role="radiogroup"]')) {
      const radios = group.findAll<HTMLButtonElement>('[role="radio"]');
      expect(radios.filter((radio) => radio.element.tabIndex === 0)).toHaveLength(1);
      radios[0]!.element.focus();
      await radios[0]!.trigger("keydown", { key: "ArrowRight" });
      await flushPromises();
      expect(document.activeElement).toBe(radios[1]!.element);
      expect(radios[1]!.attributes("aria-checked")).toBe("true");
      await radios[1]!.trigger("keydown", { key: "ArrowUp" });
      await flushPromises();
      await radios[0]!.trigger("keydown", { key: "ArrowLeft" });
      await flushPromises();
      expect(document.activeElement).toBe(radios.at(-1)!.element);
      expect(radios.at(-1)!.attributes("aria-checked")).toBe("true");
      expect(radios.filter((radio) => radio.element.tabIndex === 0)).toHaveLength(1);
    }
    wrapper.unmount();
  });

  it.each([["account", SignInView], ["SSH key", SSHKeyForm]] as const)("selects and labels %s panels using arrows, Home and End", async (_name, component) => {
    const wrapper = mount(component, { attachTo: document.body });
    const tabs = wrapper.findAll<HTMLButtonElement>('[role="tab"]');
    const assertSelection = (index: number) => {
      expect(document.activeElement).toBe(tabs[index]!.element);
      expect(tabs[index]!.attributes("aria-selected")).toBe("true");
      expect(tabs.filter((tab) => tab.element.tabIndex === 0)).toHaveLength(1);
      const panel = wrapper.get('[role="tabpanel"]');
      expect(panel.attributes("id")).toBe(tabs[index]!.attributes("aria-controls"));
      expect(panel.attributes("aria-labelledby")).toBe(tabs[index]!.attributes("id"));
    };
    tabs[0]!.element.focus();
    assertSelection(0);
    await tabs[0]!.trigger("keydown", { key: "ArrowRight" });
    assertSelection(1);
    await tabs[1]!.trigger("keydown", { key: "ArrowRight" });
    assertSelection(0);
    await tabs[0]!.trigger("keydown", { key: "End" });
    assertSelection(1);
    await tabs[1]!.trigger("keydown", { key: "Home" });
    assertSelection(0);
    await tabs[0]!.trigger("keydown", { key: "ArrowLeft" });
    assertSelection(1);
    wrapper.unmount();
  });
});
