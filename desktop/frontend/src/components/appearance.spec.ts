import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import type { createApiMock } from "../testing";

const apiMock = vi.hoisted(() => ({ current: {} as ReturnType<typeof createApiMock> }));
const eventListener = vi.hoisted(() => ({ allByName: {} as Record<string, (event: { data: unknown }) => void> }));

vi.mock("../api", async () => {
  const { createApiMock: createMock } = await import("../testing");
  apiMock.current = createMock();
  return { api: apiMock.current, getErrorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)) };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, listener: (event: { data: unknown }) => void) => {
      eventListener.allByName[name] = listener;
      return () => delete eventListener.allByName[name];
    },
  },
}));

const { applyCachedTheme, applyTheme, startThemeSync, themeStore } = await import("../theme");
const AppearanceSection = (await import("./AppearanceSection.vue")).default;
const SettingsPanel = (await import("./SettingsPanel.vue")).default;
const api = apiMock.current;

function readAppliedTheme(): { appearance?: string; color?: string } {
  return { appearance: document.documentElement.dataset.appearance, color: document.documentElement.dataset.color };
}

function findAllChecked(wrapper: ReturnType<typeof mount>): string[] {
  return wrapper.findAll('[role="radio"][aria-checked="true"]').map((radio) => radio.attributes("data-testid") ?? "");
}

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  applyTheme({ appearance: "system", color: "neutral" });
  themeStore.errorMessage = "";
  api.getTheme.mockResolvedValue({ appearance: "system", color: "neutral" });
  api.setTheme.mockImplementation((appearance: string, color: string) => Promise.resolve({ appearance, color }));
});

describe("AppearanceSection", () => {
  it("starts on System and Neutral, with System, Light and Dark and four colours", () => {
    const wrapper = mount(AppearanceSection);
    expect(wrapper.findAll('[role="radio"][data-testid^="appearance-"]').map((button) => button.text())).toEqual(["System", "Light", "Dark"]);
    expect(wrapper.findAll(".swatch-name").map((name) => name.text())).toEqual(["Neutral", "Blue", "Purple", "Green"]);
    expect(findAllChecked(wrapper)).toEqual(["appearance-system", "theme-color-neutral"]);
    expect(wrapper.find('[data-testid="theme-color-neutral"] .swatch-check').exists()).toBe(true);
  });

  it("previews a choice at once, before the backend has saved it, and saves it", async () => {
    let finishSaving: (theme: { appearance: string; color: string }) => void = () => undefined;
    api.setTheme.mockReturnValueOnce(new Promise((resolve) => (finishSaving = resolve)));
    const wrapper = mount(AppearanceSection);
    await wrapper.find('[data-testid="appearance-dark"]').trigger("click");
    expect(readAppliedTheme()).toEqual({ appearance: "dark", color: "neutral" });
    expect(findAllChecked(wrapper)).toEqual(["appearance-dark", "theme-color-neutral"]);
    expect(api.setTheme).toHaveBeenCalledWith("dark", "neutral");
    finishSaving({ appearance: "dark", color: "neutral" });
    await flushPromises();

    await wrapper.find('[data-testid="theme-color-purple"]').trigger("click");
    await flushPromises();
    expect(api.setTheme).toHaveBeenLastCalledWith("dark", "purple");
    expect(readAppliedTheme()).toEqual({ appearance: "dark", color: "purple" });
    expect(findAllChecked(wrapper)).toEqual(["appearance-dark", "theme-color-purple"]);
  });

  it("draws every colour swatch in the chosen appearance", async () => {
    const wrapper = mount(AppearanceSection);
    await wrapper.find('[data-testid="appearance-light"]').trigger("click");
    await flushPromises();
    const allSwatch = wrapper.findAll(".swatch");
    expect(allSwatch.map((swatch) => swatch.attributes("data-color"))).toEqual(["neutral", "blue", "purple", "green"]);
    expect(allSwatch.every((swatch) => swatch.attributes("data-appearance") === "light")).toBe(true);
  });

  it("goes back to the theme in use when saving fails", async () => {
    api.setTheme.mockRejectedValueOnce(new Error("could not save desktop.json"));
    const wrapper = mount(AppearanceSection);
    await wrapper.find('[data-testid="theme-color-green"]').trigger("click");
    await flushPromises();
    expect(readAppliedTheme()).toEqual({ appearance: "system", color: "neutral" });
    expect(findAllChecked(wrapper)).toEqual(["appearance-system", "theme-color-neutral"]);
    expect(wrapper.find('[data-testid="theme-error"]').text()).toBe("could not save desktop.json");
  });

  it("is found by searching for a colour or for dark mode", async () => {
    const wrapper = mount(SettingsPanel, { attachTo: document.body });
    await wrapper.find('[data-testid="settings-search"]').setValue("purple");
    expect(wrapper.find('[data-testid="appearance-section"]').isVisible()).toBe(true);
    expect(wrapper.find('[data-testid="theme-color-purple"]').isVisible()).toBe(true);
    expect(wrapper.find('[data-testid="appearance-dark"]').isVisible()).toBe(false);
    await wrapper.find('[data-testid="settings-search"]').setValue("dark mode");
    expect(wrapper.find('[data-testid="appearance-dark"]').isVisible()).toBe(true);
    wrapper.unmount();
  });
});

describe("theme sync", () => {
  it("loads the saved theme and caches it for the next window's first paint", async () => {
    api.getTheme.mockResolvedValue({ appearance: "light", color: "blue" });
    const stopThemeSync = startThemeSync();
    await flushPromises();
    expect(readAppliedTheme()).toEqual({ appearance: "light", color: "blue" });
    applyTheme({ appearance: "system", color: "neutral" });
    window.localStorage.setItem("hisar.theme", JSON.stringify({ appearance: "dark", color: "green" }));
    applyCachedTheme();
    expect(readAppliedTheme()).toEqual({ appearance: "dark", color: "green" });
    stopThemeSync();
  });

  it("follows a theme chosen in another window and ignores unknown values", async () => {
    const stopThemeSync = startThemeSync();
    await flushPromises();
    eventListener.allByName["theme-changed"]({ data: { appearance: "dark", color: "blue" } });
    expect(readAppliedTheme()).toEqual({ appearance: "dark", color: "blue" });
    eventListener.allByName["theme-changed"]({ data: { appearance: "sepia", color: "orange" } });
    expect(readAppliedTheme()).toEqual({ appearance: "system", color: "neutral" });
    stopThemeSync();
    expect(eventListener.allByName["theme-changed"]).toBeUndefined();
  });

  it("falls back to the defaults when the cache is unreadable", () => {
    window.localStorage.setItem("hisar.theme", "{not json");
    applyCachedTheme();
    expect(readAppliedTheme()).toEqual({ appearance: "system", color: "neutral" });
  });
});
