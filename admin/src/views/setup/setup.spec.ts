import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { createApp, nextTick, type App, type Component } from "vue";
import type { InstanceCheck } from "../../api";
import { bridge, type BridgeWorker, type SignUpResult } from "../../bridge/bridge";
import SetupView from "../SetupView.vue";
import ChecksStep from "./ChecksStep.vue";
import EmergencyKitStep from "./EmergencyKitStep.vue";

const flush = async () => {
  for (let index = 0; index < 5; index++) {
    await Promise.resolve();
    await nextTick();
  }
};

let app: App | undefined;
let root: HTMLElement;

function mount(component: Component, props: Record<string, unknown>): void {
  root = document.createElement("div");
  document.body.append(root);
  app = createApp(component, props);
  app.mount(root);
}

afterEach(() => {
  app?.unmount();
  app = undefined;
  root.remove();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const findButton = (text: string) => [...root.querySelectorAll("button")].find((button) => button.textContent?.trim() === text)!;

function tick(name: string): void {
  const checkbox = root.querySelector<HTMLInputElement>(`input[name="${name}"]`)!;
  checkbox.checked = true;
  checkbox.dispatchEvent(new Event("change"));
}

describe("EmergencyKitStep", () => {
  const signedUp: SignUpResult = {
    token: "token",
    accountId: "account",
    deviceId: "device",
    secretKey: "H1-ABCDEF-GHIJK-LMNOP-QRSTU-VWXYZ-ABCDEF",
    emergencyKitPdf: new TextEncoder().encode("%PDF-1.4"),
  };

  beforeEach(() => {
    Object.assign(URL, { createObjectURL: vi.fn(() => "blob:kit"), revokeObjectURL: vi.fn() });
  });

  it("keeps Continue disabled until the kit is downloaded and the admin says it is saved", async () => {
    const clickLink = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    const onContinue = vi.fn();
    mount(EmergencyKitStep, { result: signedUp, onContinue });
    await flush();
    expect(root.querySelector('[data-testid="secret-key"]')?.textContent).toBe(signedUp.secretKey);
    expect(findButton("Continue").disabled).toBe(true);

    tick("isSaved");
    await flush();
    expect(findButton("Continue").disabled).toBe(true);

    findButton("Download Emergency Kit").click();
    await flush();
    expect(clickLink).toHaveBeenCalledOnce();
    expect((clickLink.mock.contexts[0] as HTMLAnchorElement).download).toBe("Hisar Emergency Kit.pdf");
    expect(findButton("Continue").disabled).toBe(false);

    findButton("Continue").click();
    expect(onContinue).toHaveBeenCalledOnce();
  });

  it("keeps Continue disabled after a download until the admin ticks the box", async () => {
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    mount(EmergencyKitStep, { result: signedUp });
    findButton("Download Emergency Kit").click();
    await flush();
    expect(findButton("Continue").disabled).toBe(true);
  });

  it("shows the Secret Key and lets the admin go on after writing it down when the kit failed", async () => {
    mount(EmergencyKitStep, { result: { ...signedUp, emergencyKitPdf: undefined, emergencyKitError: "emergencykit: font" } });
    await flush();
    expect(root.querySelector('[data-testid="secret-key"]')?.textContent).toBe(signedUp.secretKey);
    expect(root.querySelector('[role="alert"]')?.textContent).toContain("Write down the Secret Key above.");
    expect(findButton("Continue").disabled).toBe(true);
    tick("isSaved");
    await flush();
    expect(findButton("Continue").disabled).toBe(false);
  });

  it("says the Personal vault failed and still shows the Secret Key", async () => {
    mount(EmergencyKitStep, { result: { ...signedUp, personalVaultError: "create personal vault: internal error" } });
    await flush();
    expect(root.querySelector('[data-testid="secret-key"]')?.textContent).toBe(signedUp.secretKey);
    expect(root.querySelector('[data-testid="personal-vault-error"]')?.textContent).toContain(
      "the Personal vault could not be created (create personal vault: internal error). You can create a vault later from the Hisar apps.",
    );
  });

  it("asks before the page unloads until the kit is saved", async () => {
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    mount(EmergencyKitStep, { result: signedUp });
    await flush();
    const unload = () => {
      const event = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(event);
      return event.defaultPrevented;
    };
    expect(unload()).toBe(true);
    findButton("Download Emergency Kit").click();
    tick("isSaved");
    await flush();
    expect(unload()).toBe(false);
    expect(findButton("Continue").disabled).toBe(false);
  });
});

describe("ChecksStep", () => {
  let fetchMock: Mock<typeof fetch>;
  let checkAnswers: InstanceCheck[][];

  beforeEach(() => {
    checkAnswers = [
      [
        { id: "public-url", title: "Public URL", status: "ok", message: "" },
        { id: "migrations", title: "Migrations", status: "failed", message: "1 migration is not applied." },
        { id: "email", title: "Email", status: "warning", message: "Email is not set up. Invites from the apps will fail." },
      ],
      [
        { id: "public-url", title: "Public URL", status: "ok", message: "" },
        { id: "migrations", title: "Migrations", status: "ok", message: "" },
        { id: "email", title: "Email", status: "warning", message: "Email is not set up. Invites from the apps will fail." },
      ],
    ];
    fetchMock = vi.fn<typeof fetch>(async (_url, init) => {
      if (init?.method === "POST") {
        return new Response(JSON.stringify({ state: "needsInvitePolicy" }), { status: 200 });
      }
      return new Response(JSON.stringify({ checks: checkAnswers.shift() }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  it("blocks Continue on a failed check, names it, and allows it with warnings only", async () => {
    const onContinue = vi.fn();
    mount(ChecksStep, { onContinue });
    await flush();
    expect(root.querySelector('[data-testid="check-migrations"]')?.className).toContain("failed");
    expect(root.textContent).toContain("Fix Migrations first, then check again.");
    expect(findButton("Continue").disabled).toBe(true);

    findButton("Check again").click();
    await flush();
    expect(root.querySelector('[data-testid="check-email"]')?.className).toContain("warning");
    expect(findButton("Continue").disabled).toBe(false);

    root.querySelector("form")!.dispatchEvent(new Event("submit"));
    await flush();
    const [url, request] = fetchMock.mock.calls.at(-1)! as [string, RequestInit];
    expect([url, request.method]).toEqual(["/api/v1/admin/setup/checks", "POST"]);
    expect(onContinue).toHaveBeenCalledOnce();
  });
});

describe("SetupView", () => {
  function answerSetupState(state: string): void {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ state }), { status: 200 })),
    );
  }

  it("leaves for the dashboard once setup is done", async () => {
    answerSetupState("done");
    const onLeave = vi.fn();
    mount(SetupView, { onLeave });
    await flush();
    expect(onLeave).toHaveBeenCalledOnce();
    expect(root.textContent).not.toContain("Create the first admin");
  });

  it("asks for the link in the server log when the URL holds no setup invite", async () => {
    answerSetupState("needsAdmin");
    window.history.replaceState(null, "", "/admin/setup");
    mount(SetupView, {});
    await flush();
    expect(root.textContent).toContain("This setup link is no longer valid. Use the link in the latest server log.");
    expect(root.querySelector("form")).toBeNull();
  });

  it("reads a setup invite pasted into the same tab", async () => {
    answerSetupState("needsAdmin");
    vi.spyOn(bridge, "start").mockReturnValue({} as BridgeWorker);
    window.history.replaceState(null, "", "/admin/setup");
    mount(SetupView, {});
    await flush();
    expect(root.querySelector("form")).toBeNull();
    window.history.replaceState(null, "", "/admin/setup#new-token");
    window.dispatchEvent(new HashChangeEvent("hashchange"));
    await flush();
    expect(root.querySelector("form")).not.toBeNull();
    expect(root.textContent).not.toContain("This setup link is no longer valid.");
  });

  it("zeroes the Emergency Kit's bytes once the admin goes on from the kit", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ state: "needsAdmin", checks: [] }), { status: 200 })),
    );
    vi.spyOn(bridge, "start").mockReturnValue({} as BridgeWorker);
    const emergencyKitPdf = new TextEncoder().encode("%PDF-1.4 H1-ABCDEF");
    const secretKey = "H1-ABCDEF-GHIJK-LMNOP-QRSTU-VWXYZ-ABCDEF";
    vi.spyOn(bridge, "signUp").mockResolvedValue({ token: "token", accountId: "account", deviceId: "device", secretKey, emergencyKitPdf });
    Object.assign(URL, { createObjectURL: vi.fn(() => "blob:kit"), revokeObjectURL: vi.fn() });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    window.history.replaceState(null, "", "/admin/setup#setup-token");
    mount(SetupView, {});
    await flush();
    for (const [name, value] of Object.entries({ name: "Ada", email: "ada@example.com", password: "a long password", repeatedPassword: "a long password" })) {
      const input = root.querySelector<HTMLInputElement>(`input[name="${name}"]`)!;
      input.value = value;
      input.dispatchEvent(new Event("input"));
    }
    root.querySelector("form")!.dispatchEvent(new Event("submit"));
    await flush();
    findButton("Download Emergency Kit").click();
    tick("isSaved");
    await flush();

    findButton("Continue").click();
    await flush();
    expect(root.textContent).not.toContain(secretKey);
    expect(emergencyKitPdf.every((byte) => byte === 0)).toBe(true);
  });
});
