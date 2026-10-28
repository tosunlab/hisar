import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount, type DOMWrapper } from "@vue/test-utils";
import { nextTick } from "vue";
import type { VaultSummary } from "../api";
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

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));

const { store } = await import("../store");
const MainView = (await import("./MainView.vue")).default;

const personalVault: VaultSummary = { id: "personal", name: "Personal", type: "personal", role: "owner", itemCount: 0 };

// jsdom has no PointerEvent, so the drag sends pointer events as MouseEvents, which carry clientX.
async function drag(handle: DOMWrapper<Element>, fromX: number, toX: number): Promise<void> {
  for (const [type, clientX] of [["pointerdown", fromX], ["pointermove", toX], ["pointerup", toX]] as const) {
    handle.element.dispatchEvent(new MouseEvent(type, { clientX, bubbles: true }));
  }
  await nextTick();
}

beforeEach(() => {
  window.localStorage.clear();
  Object.assign(store, {
    status: createStatus("unlocked"),
    allVault: [personalVault],
    selectedVaultID: "personal",
    allItem: [],
    selectedItem: null,
    isFavoritesSelected: false,
    editorMode: "view",
    searchQuery: "",
    isSettingsOpen: false,
  });
  document.body.innerHTML = "";
});

describe("Resizable columns", () => {
  it("drags the sidebar and the list within their limits and keeps the widths on this device", async () => {
    const wrapper = mount(MainView, { attachTo: document.body });
    const sidebarHandle = wrapper.find('[data-testid="resize-sidebar"]');
    await drag(sidebarHandle, 228, 300);
    const listHandle = wrapper.find('[data-testid="resize-list"]');
    await drag(listHandle, 600, 100);
    expect(sidebarHandle.attributes("aria-valuenow")).toBe("300");
    expect(listHandle.attributes("aria-valuenow")).toBe("240");
    expect(JSON.parse(window.localStorage.getItem("hisar.mainWindow.columnWidths") ?? "null")).toEqual({ sidebar: 300, list: 240 });
    wrapper.unmount();

    const reopened = mount(MainView, { attachTo: document.body });
    expect(reopened.get('[data-testid="resize-sidebar"]').attributes("aria-valuenow")).toBe("300");
    expect(reopened.get('[data-testid="resize-list"]').attributes("aria-valuenow")).toBe("240");
    reopened.unmount();
  });

  it("resizes with the arrow keys up to the maximum", async () => {
    window.localStorage.setItem("hisar.mainWindow.columnWidths", JSON.stringify({ sidebar: 350, list: "wide" }));
    const wrapper = mount(MainView, { attachTo: document.body });
    const sidebarHandle = wrapper.find('[data-testid="resize-sidebar"]');
    expect(sidebarHandle.attributes("role")).toBe("separator");
    await sidebarHandle.trigger("keydown", { key: "ArrowRight" });
    expect(sidebarHandle.attributes("aria-valuenow")).toBe("360");
    expect(wrapper.get('[data-testid="resize-list"]').attributes("aria-valuenow")).toBe("300");
    await sidebarHandle.trigger("keydown", { key: "ArrowLeft" });
    expect(sidebarHandle.attributes("aria-valuenow")).toBe("344");
    expect(JSON.parse(window.localStorage.getItem("hisar.mainWindow.columnWidths") ?? "null")).toEqual({ sidebar: 344, list: 300 });
    wrapper.unmount();
  });
});
