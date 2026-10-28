import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { api } from "../api";
import { createEmergencyKit, createStatus } from "../testing";
import { store } from "../store";
import { getSignOutCopy } from "../util";
import EmergencyKitBanner from "./EmergencyKitBanner.vue";
import MainView from "./MainView.vue";
import SignInView from "./SignInView.vue";

vi.mock("../api", async () => {
  // Vitest runs this factory before static fixture imports initialize.
  const { createApiMock } = await import("../testing");
  return { api: createApiMock(), getErrorMessage: String };
});
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => undefined } }));
enableAutoUnmount(afterEach);
beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { status: { ...createStatus("unlocked"), emergencyKit: createEmergencyKit() }, onboardingStep: "", pendingRecovery: [], pendingInvite: null, isRecoveryOpen: false, allVault: [], allItem: [], allFavorite: [], isSettingsOpen: false, isBusy: false, errorMessage: "", editorMode: "view", selectedItem: null });
});

it.each([
  ["new", "Save your Emergency Kit", "Save a copy you can reach without this Mac."],
  ["3-days", "Your Emergency Kit still isn't saved", "Losing access to your signed-in devices could lock you out."],
  ["14-days", "Save your Emergency Kit now", "It's been at least 2 weeks."],
])("shows the %s reminder without taking focus", (reminder, title, body) => {
  store.status.emergencyKit = createEmergencyKit({ reminder });
  const focus = document.activeElement;
  const wrapper = mount(EmergencyKitBanner);
  expect(wrapper.get("h2").text()).toBe(title);
  expect(wrapper.text()).toContain(body);
  expect(wrapper.find('[role="alert"]').exists()).toBe(false);
  expect(document.activeElement).toBe(focus);
});

it("uses confirmation copy for delivered kits rather than saying they are unsaved", () => {
  store.status.emergencyKit = createEmergencyKit({ savedBy: "file", reminder: "14-days" });
  const wrapper = mount(EmergencyKitBanner);
  expect(wrapper.get("h2").text()).toBe("Confirm your Emergency Kit");
  expect(wrapper.text()).toContain("Emergency Kit saved.");
  expect(wrapper.text()).toContain("Continue to confirmation");
  expect(wrapper.classes()).not.toContain("urgent");
});

it("warns that a replacement kit supersedes the old one", () => {
  store.status.emergencyKit = createEmergencyKit({ kind: "replace-secret-key" });
  const wrapper = mount(EmergencyKitBanner);
  expect(wrapper.text()).toContain("Your old Emergency Kit no longer works.");
});

it("opens the one kit screen and hides only until the backend's next lock", async () => {
  const wrapper = mount(EmergencyKitBanner);
  await wrapper.get("button").trigger("click");
  expect(store.onboardingStep).toBe("kit");
  vi.mocked(api.getStatus).mockResolvedValue({ ...store.status, emergencyKit: createEmergencyKit({ isReminderHidden: true }) });
  await wrapper.get('[aria-label="Hide until Hisar locks"]').trigger("click");
  await flushPromises();
  expect(api.hideEmergencyKitReminder).toHaveBeenCalledTimes(1);
  expect(wrapper.find("section").exists()).toBe(false);
  store.status.emergencyKit = createEmergencyKit();
  await wrapper.vm.$nextTick();
  expect(wrapper.find("section").exists()).toBe(true);
});

it("places the banner first in the item-list column, never in the sidebar", () => {
  const wrapper = mount(MainView);
  expect(wrapper.get('section.list').element.firstElementChild?.getAttribute("data-testid")).toBe("emergency-kit-banner");
  expect(wrapper.find('.sidebar [data-testid="emergency-kit-banner"]').exists()).toBe(false);
});

it.each([false, true])("distinguishes missing, unsaved and unconfirmed kits when locked=%s", locked => {
  expect(getSignOutCopy(locked).message).not.toContain("You haven't");
  expect(getSignOutCopy(locked, createEmergencyKit()).message).toContain("You haven't saved this account's Emergency Kit yet.");
  const delivered = getSignOutCopy(locked, createEmergencyKit({ savedBy: "file" })).message;
  expect(delivered).toContain("You haven't confirmed this account's Emergency Kit yet.");
  expect(delivered).toContain("offline copy of your vaults");
  expect(delivered).toContain("master password");
  expect(delivered).toContain("protected unfinished setup stays");
  if (locked) expect(delivered).toContain("Cancel, unlock Hisar and finish confirming");
});

it("keeps credentials when opening and closing lost-key help", async () => {
  store.status = createStatus("signed-out");
  const wrapper = mount(SignInView, { attachTo: document.body });
  await wrapper.get('input[name="secretKey"]').setValue("H1-RETAINED");
  await wrapper.get('input[name="password"]').setValue("master password");
  await wrapper.get('[data-testid="secret-key-note"]').trigger("click");
  expect(wrapper.get('[role="dialog"]').text()).toContain("protected unfinished setup you can resume");
  await wrapper.findAll("button").find(button => button.text() === "Done")!.trigger("click");
  expect((wrapper.get('input[name="secretKey"]').element as HTMLInputElement).value).toBe("H1-RETAINED");
  expect((wrapper.get('input[name="password"]').element as HTMLInputElement).value).toBe("master password");
});
