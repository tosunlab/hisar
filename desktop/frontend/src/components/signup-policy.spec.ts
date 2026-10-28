import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { api } from "../api";
import { createStatus } from "../testing";
import { store, openInviteLink } from "../store";
import SignInView from "./SignInView.vue";

vi.mock("../api", async () => {
  // Vitest runs this factory before static fixture imports initialize.
  const { createApiMock } = await import("../testing");
  return { api: createApiMock(), getErrorMessage: (error: unknown) => error instanceof Error ? error.message : String(error) };
});
enableAutoUnmount(afterEach);
beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(store, { status: createStatus("signed-out"), pendingInvite: null, pendingRecovery: [], onboardingStep: "", isRecoveryOpen: false, isBusy: false, errorMessage: "" });
  vi.mocked(api.getSignUpPolicy).mockImplementation(async server => ({ server, host: server.replace(/^https:\/\//, ""), mode: "open" }));
});

async function createForm(mode = "open"): Promise<VueWrapper> {
  vi.mocked(api.getSignUpPolicy).mockImplementation(async server => ({ server, host: server.replace(/^https:\/\//, ""), mode }));
  const wrapper = mount(SignInView, { attachTo: document.body });
  await wrapper.get('#account-tab-create-account').trigger("click");
  await flushPromises();
  return wrapper;
}
async function fillAccount(wrapper: VueWrapper): Promise<void> {
  await wrapper.get('input[name="name"]').setValue("Ada");
  await wrapper.get('input[name="email"]').setValue("ada@example.com");
  await wrapper.get('input[name="password"]').setValue("orange cactus harbor lantern");
  await wrapper.get('input[name="repeatedPassword"]').setValue("orange cactus harbor lantern");
  await flushPromises();
}

it("hides the invite field only after a confirmed open answer", async () => {
  const wrapper = await createForm();
  expect(wrapper.find('input[name="inviteCode"]').exists()).toBe(false);
  expect(wrapper.find('input[name="name"]').exists()).toBe(true);
});

it.each(["invite", "setup"])("asks for an invitation before account fields on %s", async mode => {
  const wrapper = await createForm(mode);
  expect(wrapper.find('input[name="name"]').exists()).toBe(false);
  expect(wrapper.get('input[name="inviteCode"]').attributes("required")).toBeDefined();
  await wrapper.get('input[name="inviteCode"]').setValue("bare_CODE");
  expect(wrapper.get('[data-testid="invitation-card"]').text()).toContain("hisar.test");
  expect(wrapper.find('input[name="name"]').exists()).toBe(true);
  await fillAccount(wrapper);
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(api.signUp).toHaveBeenCalledWith("https://hisar.test/invite/bare_CODE", "ada@example.com", "Ada", "orange cactus harbor lantern");
});

it("keeps an optional invitation and a usable form for an old or unknown server", async () => {
  const wrapper = await createForm("unknown");
  expect(wrapper.get('input[name="inviteCode"]').attributes("required")).toBeUndefined();
  expect(wrapper.find('input[name="name"]').exists()).toBe(true);
  expect(wrapper.text()).toContain("This server doesn't say whether it needs an invitation.");
});

it("keeps an invite field and Try again after a discovery error", async () => {
  vi.mocked(api.getSignUpPolicy).mockRejectedValueOnce(new Error("Hisar can't reach hisar.test. Check your internet connection and the server address."));
  const wrapper = mount(SignInView);
  await wrapper.get('#account-tab-create-account').trigger("click");
  await flushPromises();
  expect(wrapper.find('input[name="inviteCode"]').exists()).toBe(true);
  expect(wrapper.find('input[name="name"]').exists()).toBe(false);
  await wrapper.findAll("button").find(button => button.text() === "Try again")!.trigger("click");
  await flushPromises();
  expect(wrapper.find('input[name="inviteCode"]').exists()).toBe(false);
});

it("ignores two servers answered in reverse order and clears the old error", async () => {
  const first = Promise.withResolvers<{ server: string; host: string; mode: string }>();
  const second = Promise.withResolvers<{ server: string; host: string; mode: string }>();
  vi.mocked(api.getSignUpPolicy).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const wrapper = mount(SignInView);
  await wrapper.get('#account-tab-create-account').trigger("click");
  store.errorMessage = "Old server refused.";
  await wrapper.get('[data-testid="use-different-server"]').trigger("click");
  await wrapper.get('input[name="server"]').setValue("other.test/sub");
  await wrapper.get('input[name="server"]').trigger("blur");
  expect(store.errorMessage).toBe("");
  second.resolve({ server: "https://other.test/sub", host: "other.test/sub", mode: "open" });
  await flushPromises();
  first.resolve({ server: "https://hisar.test", host: "hisar.test", mode: "invite" });
  await flushPromises();
  expect(wrapper.find('input[name="inviteCode"]').exists()).toBe(false);
  expect(wrapper.get('[data-testid="create-account-server"]').text()).toContain("other.test/sub");
});

it("shows an invitation card on an open server and removes it when the server changes", async () => {
  const wrapper = await createForm();
  openInviteLink({ invite: "https://first.test/sub/invite/abc", server: "https://first.test/sub", email: "ada@example.com", invitation: "" });
  await flushPromises();
  expect(wrapper.get('[data-testid="invitation-card"]').text()).toContain("ada@example.com");
  await wrapper.get('input[name="name"]').setValue("Ada");
  await wrapper.get('input[name="password"]').setValue("orange cactus harbor lantern");
  await wrapper.get('[data-testid="use-different-server"]').trigger("click");
  await wrapper.get('input[name="server"]').setValue("other.test");
  await wrapper.get('input[name="server"]').trigger("blur");
  await flushPromises();
  expect(wrapper.find('[data-testid="invitation-card"]').exists()).toBe(false);
  expect(wrapper.text()).toContain("This invitation is for first.test/sub, so Hisar removed it.");
  expect((wrapper.get('input[name="email"]').element as HTMLInputElement).value).toBe("ada@example.com");
  expect((wrapper.get('input[name="name"]').element as HTMLInputElement).value).toBe("Ada");
  expect((wrapper.get('input[name="password"]').element as HTMLInputElement).value).toBe("orange cactus harbor lantern");
});

it("brings back the invite field after stale open policy without losing typed values", async () => {
  const wrapper = await createForm();
  await fillAccount(wrapper);
  vi.mocked(api.signUpWithoutInvite).mockRejectedValueOnce(new Error("Sign-ups are not open on this server. Ask an admin for an invitation."));
  vi.mocked(api.getSignUpPolicy).mockImplementation(async server => ({ server, host: "hisar.test", mode: "invite" }));
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(wrapper.find('input[name="inviteCode"]').exists()).toBe(true);
  for (const [name, value] of [["name", "Ada"], ["email", "ada@example.com"], ["password", "orange cactus harbor lantern"], ["repeatedPassword", "orange cactus harbor lantern"]]) {
    expect((wrapper.get(`input[name="${name}"]`).element as HTMLInputElement).value).toBe(value);
  }
  expect(wrapper.text()).toContain("Sign-ups on hisar.test need an invitation now.");
});

it("blocks duplicate requests while creating and preserves credentials across help", async () => {
  const wrapper = await createForm();
  await fillAccount(wrapper);
  vi.mocked(api.signUpWithoutInvite).mockReturnValueOnce(new Promise(() => undefined));
  await wrapper.get("form").trigger("submit");
  await wrapper.get("form").trigger("submit");
  expect(api.signUpWithoutInvite).toHaveBeenCalledTimes(1);
  expect(wrapper.get('button[type="submit"]').attributes("disabled")).toBeDefined();
});

it("does not hide a confirmed form when the unchanged server field loses focus", async () => {
  const wrapper = await createForm();
  await wrapper.get('[data-testid="use-different-server"]').trigger("click");
  await wrapper.get('input[name="server"]').trigger("blur");
  expect(api.getSignUpPolicy).toHaveBeenCalledTimes(1);
  await wrapper.get("form").trigger("submit");
  expect(document.activeElement?.getAttribute("name")).toBe("name");
  expect(wrapper.text()).toContain("Enter your name.");
});

it("does not silently discard a malformed optional invitation", async () => {
  const wrapper = await createForm("unknown");
  await fillAccount(wrapper);
  await wrapper.get('input[name="inviteCode"]').setValue("not an invite");
  await wrapper.get("form").trigger("submit");
  expect(api.signUpWithoutInvite).not.toHaveBeenCalled();
  expect(document.activeElement?.getAttribute("name")).toBe("inviteCode");
  expect(wrapper.text()).toContain("Enter your invite link or code.");
});
