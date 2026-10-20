import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PasskeyPromptCall, PasskeyPromptState } from "../shared/passkey";
import { HisarError } from "../shared/protocol";
import { createPromptChannel, type PromptChannel, type PromptPort } from "./api";
import PasskeyPrompt from "./PasskeyPrompt.vue";

const saveState: PasskeyPromptState = {
  view: "save",
  kind: "create",
  relyingPartyID: "example.com",
  userName: "alex@example.com",
  allPasskey: [],
  allLogin: [{ vaultID: "v1", itemID: "i1", title: "Example", username: "alex@example.com", vaultName: "Personal", isPersonal: true }],
};

const chooseState: PasskeyPromptState = {
  view: "choose",
  kind: "get",
  relyingPartyID: "example.com",
  userName: "",
  allPasskey: [
    { credentialId: "AQ", vaultID: "v1", itemID: "i1", title: "Example", userName: "alex@example.com", userDisplayName: "Alex" },
    { credentialId: "Ag", vaultID: "v1", itemID: "i2", title: "Example (work)", userName: "alex@work.example", userDisplayName: "" },
  ],
  allLogin: [],
};

function createChannel(answer: (call: PasskeyPromptCall) => unknown): PromptChannel & { allCall: PasskeyPromptCall[] } {
  const allCall: PasskeyPromptCall[] = [];
  return {
    allCall,
    call: vi.fn(async (call: PasskeyPromptCall) => {
      allCall.push(call);
      return answer(call);
    }) as never,
  };
}

async function mountPrompt(channel: PromptChannel, props: Record<string, unknown> = {}) {
  const wrapper = mount(PasskeyPrompt, { props: { channel, isEventTrusted: () => true, ...props }, attachTo: document.body });
  await flushPromises();
  return wrapper;
}

afterEach(() => {
  vi.useRealTimers();
  document.body.innerHTML = "";
});

describe("passkey prompt page", () => {
  it("offers to save the passkey in a new login or a matching one, preselecting the same user", async () => {
    const channel = createChannel((call) => (call.method === "getState" ? saveState : null));
    const wrapper = await mountPrompt(channel);
    expect(wrapper.get('[data-testid="passkey-heading"]').text()).toBe("Save passkey");
    expect(wrapper.get('[data-testid="passkey-site"]').text()).toBe("example.com");
    expect(wrapper.get('[data-testid="passkey-user-name"]').text()).toBe("alex@example.com");
    const allOption = wrapper.findAll('[data-testid="passkey-login-option"]');
    expect(allOption.map((option) => option.text())).toEqual(["New loginin your Personal vault", "Examplealex@example.com · Personal"]);
    expect((allOption[1].get("input").element as HTMLInputElement).checked).toBe(true);
    await wrapper.get('[data-testid="save-passkey"]').trigger("click");
    await flushPromises();
    expect(channel.allCall).toContainEqual({ method: "savePasskey", vaultID: "v1", itemID: "i1" });
    await allOption[0].get("input").setValue(true);
    await wrapper.get('[data-testid="save-passkey"]').trigger("click");
    await flushPromises();
    expect(channel.allCall).toContainEqual({ method: "savePasskey" });
  });

  it("lists the passkeys and signs in with the chosen one", async () => {
    const channel = createChannel((call) => (call.method === "getState" ? chooseState : null));
    const wrapper = await mountPrompt(channel);
    expect(wrapper.get('[data-testid="passkey-heading"]').text()).toBe("Sign in with a passkey");
    const allChoice = wrapper.findAll('[data-testid="passkey-choice"]');
    expect(allChoice.map((choice) => choice.text())).toEqual(["Ealex@example.comExample", "Ealex@work.exampleExample (work)"]);
    await allChoice[1].trigger("click");
    await flushPromises();
    expect(channel.allCall).toContainEqual({ method: "choosePasskey", credentialId: "Ag", vaultID: "v1", itemID: "i2" });
  });

  it("ignores untrusted clicks and clicks before the prompt was visible long enough", async () => {
    const channel = createChannel((call) => (call.method === "getState" ? chooseState : null));
    const untrusted = await mountPrompt(channel, { isEventTrusted: () => false });
    await untrusted.get('[data-testid="passkey-choice"]').trigger("click");
    await untrusted.get('[data-testid="use-other-device"]').trigger("click");
    untrusted.unmount();
    const observe = vi.fn();
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe = observe;
        disconnect = vi.fn();
      },
    );
    const tracked = await mountPrompt(channel, { isVisibilityTracked: true });
    await tracked.get('[data-testid="passkey-choice"]').trigger("click");
    await flushPromises();
    expect(observe).toHaveBeenCalled();
    expect(channel.allCall.filter((call) => call.method === "choosePasskey" || call.method === "useOtherDevice")).toEqual([]);
    vi.unstubAllGlobals();
  });

  it("unlocks Hisar and waits for it", async () => {
    vi.useFakeTimers();
    let state: PasskeyPromptState = { ...saveState, view: "locked" };
    const channel = createChannel((call) => {
      if (call.method === "unlock") state = saveState;
      return call.method === "getState" ? state : null;
    });
    const wrapper = await mountPrompt(channel, { pollIntervalMilliseconds: 50 });
    expect(wrapper.get('[data-testid="passkey-heading"]').text()).toBe("Hisar is locked");
    await wrapper.get('[data-testid="unlock-hisar"]').trigger("click");
    await flushPromises();
    expect(wrapper.get('[data-testid="unlock-hisar"]').text()).toBe("Waiting for Hisar…");
    await vi.advanceTimersByTimeAsync(60);
    await flushPromises();
    expect(wrapper.find('[data-testid="save-passkey"]').exists()).toBe(true);
  });

  it("confirms excluded credentials, closes, and offers another device", async () => {
    const channel = createChannel((call) => (call.method === "getState" ? { ...saveState, view: "excluded" } : null));
    const wrapper = await mountPrompt(channel);
    expect(wrapper.get('[data-testid="passkey-excluded"]').text()).toContain("already has a passkey");
    await wrapper.get('[data-testid="confirm-excluded"]').trigger("click");
    await wrapper.get('[data-testid="use-other-device"]').trigger("click");
    await wrapper.get('[data-testid="passkey-close"]').trigger("click");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await flushPromises();
    expect(channel.allCall.map((call) => call.method).filter((method) => method !== "getState" && method !== "resize")).toEqual([
      "confirmExcluded",
      "useOtherDevice",
      "dismiss",
      "dismiss",
    ]);
  });

  it("shows desktop errors and reloads the state", async () => {
    const channel = createChannel((call) => {
      if (call.method === "choosePasskey") throw new HisarError("invalid-request", "The Hisar prompt was not fully visible. Try again");
      return call.method === "getState" ? chooseState : null;
    });
    const wrapper = await mountPrompt(channel);
    await wrapper.get('[data-testid="passkey-choice"]').trigger("click");
    await flushPromises();
    expect(wrapper.get('[data-testid="passkey-error"]').text()).toBe("The Hisar prompt was not fully visible. Try again");
    expect(channel.allCall.filter((call) => call.method === "getState")).toHaveLength(2);
  });
});

describe("prompt channel", () => {
  function createPort(): PromptPort & { allSent: unknown[]; deliver: (message: unknown) => void; disconnect: () => void } {
    const allSent: unknown[] = [];
    const allListener: ((message: unknown) => void)[] = [];
    const allDisconnect: (() => void)[] = [];
    return {
      allSent,
      postMessage: (message) => allSent.push(message),
      onMessage: { addListener: (listener) => allListener.push(listener) },
      onDisconnect: { addListener: (listener) => allDisconnect.push(listener) },
      deliver: (message) => allListener.forEach((listener) => listener(message)),
      disconnect: () => allDisconnect.forEach((listener) => listener()),
    };
  }

  it("matches answers to calls and turns errors into HisarErrors", async () => {
    const port = createPort();
    let count = 0;
    const channel = createPromptChannel(port, "session-1", () => `call-${++count}`);
    const first = channel.call({ method: "getState" });
    const second = channel.call({ method: "choosePasskey", credentialId: "AQ", vaultID: "v1", itemID: "i1" });
    expect(port.allSent).toEqual([
      { method: "getState", callID: "call-1", sessionID: "session-1" },
      { method: "choosePasskey", credentialId: "AQ", vaultID: "v1", itemID: "i1", callID: "call-2", sessionID: "session-1" },
    ]);
    port.deliver({ callID: "call-2", ok: false, error: { code: "locked", message: "Hisar is locked" } });
    port.deliver({ callID: "call-1", ok: true, result: { view: "save" } });
    expect(await first).toEqual({ view: "save" });
    await expect(second).rejects.toMatchObject({ code: "locked", message: "Hisar is locked" });
    const pending = channel.call({ method: "dismiss" });
    port.disconnect();
    await expect(pending).rejects.toThrow("This passkey request has ended");
    await expect(channel.call({ method: "dismiss" })).rejects.toThrow("This passkey request has ended");
  });
});
