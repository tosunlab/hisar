import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createStatus } from "../testing";
import { store, openInviteLink } from "../store";
import SignInView from "./SignInView.vue";

vi.mock("../api", async () => {
  // Vitest runs this factory before static fixture imports initialize.
  const { createApiMock } = await import("../testing");
  return { api: createApiMock(), getErrorMessage: String };
});
enableAutoUnmount(afterEach);
beforeEach(() => Object.assign(store, { status: { ...createStatus("signed-out"), server: "", email: "" }, pendingInvite: null, pendingRecovery: [], isBusy: false, errorMessage: "" }));

it("offers account creation before credentials on a fresh device", async () => {
  const wrapper = mount(SignInView);
  expect(wrapper.get("h1").text()).toBe("Welcome to Hisar");
  expect(wrapper.find("input").exists()).toBe(false);
  await wrapper.findAll("button").find(button => button.text() === "Create account")!.trigger("click");
  await flushPromises();
  expect(wrapper.get("h1").text()).toBe("Create account");
  expect(wrapper.text()).toContain("Next, Hisar creates your Secret Key. You don't choose one.");
});

it("opens Sign in for someone who already has an account", async () => {
  const wrapper = mount(SignInView);
  await wrapper.findAll("button").find(button => button.text() === "I already have an account")!.trigger("click");
  expect(wrapper.get("h1").text()).toBe("Sign in");
  expect(wrapper.find('input[name="secretKey"]').exists()).toBe(true);
});

it("welcomes back a previously signed-in device and bypasses Welcome for pending recovery", () => {
  store.status.server = "https://hisar.test";
  const returning = mount(SignInView);
  expect(returning.get("h1").text()).toBe("Welcome back");
  returning.unmount();
  store.status.server = "";
  store.pendingRecovery = [{ operationId: "one" } as typeof store.pendingRecovery[number]];
  expect(mount(SignInView).get("h1").text()).toBe("Sign in");
});

it("takes an invite's email independently of its source and skips Welcome", async () => {
  openInviteLink({ invite: "CODE", server: "https://hisar.test/sub", email: "ada@example.com", invitation: "" });
  const wrapper = mount(SignInView);
  await flushPromises();
  expect(wrapper.get("h1").text()).toBe("Create account");
  expect(wrapper.get('[data-testid="invitation-card"]').text()).toContain("Invitation for ada@example.com");
  expect(wrapper.get('[data-testid="invited-email"]').text()).toBe("ada@example.com");
  expect(wrapper.find('input[name="email"]').exists()).toBe(false);
  expect(wrapper.get('[data-testid="create-account-server"]').text()).toContain("hisar.test/sub");
});
