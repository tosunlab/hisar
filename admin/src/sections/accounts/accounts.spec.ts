import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { createApp, type App } from "vue";
import { auth } from "../../api";
import { bridge, type ReauthenticationProof } from "../../bridge/bridge";
import { currentPath } from "../../navigation";
import AccountsView from "./AccountsView.vue";
import AccountView from "./AccountView.vue";
import { formatActivity, formatDeviceCount, readAccountId, type AdminAccount, type AdminAccountDetail, type AdminDevice } from "./accounts";

vi.mock("../../bridge/bridge", () => ({ bridge: { reauthenticate: vi.fn() } }));

const reauthenticate = bridge.reauthenticate as Mock<typeof bridge.reauthenticate>;

function createAccount(email: string, overrides: Partial<AdminAccount> = {}): AdminAccount {
  return {
    id: email.slice(0, 6).toUpperCase(),
    email,
    name: email.split("@")[0],
    role: "member",
    state: "active",
    createdAt: "2026-10-01T12:00:00Z",
    lastSeenAt: "2026-10-18T09:00:00Z",
    deviceCount: 1,
    vaultCount: 2,
    ...overrides,
  };
}

const allAccount = [createAccount("admin@example.com", { id: "ADMIN1", role: "admin" }), createAccount("ada@example.com"), createAccount("bob@example.com")];

function answer(status: number, body?: unknown): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

let app: App | undefined;
let root: HTMLElement;
let fetchMock: Mock<typeof fetch>;

function mount(component: Parameters<typeof createApp>[0], props?: Record<string, unknown>): void {
  root = document.createElement("div");
  document.body.append(root);
  app = createApp(component, props);
  app.mount(root);
}

function findButton(label: string): HTMLButtonElement {
  const button = [...root.querySelectorAll("button")].find((candidate) => candidate.textContent?.trim() === label);
  if (!button) {
    throw new Error(`no ${label} button`);
  }
  return button;
}

function type(name: string, value: string): void {
  const input = root.querySelector<HTMLInputElement>(`input[name="${name}"]`)!;
  input.value = value;
  input.dispatchEvent(new Event("input"));
}

beforeEach(() => {
  auth.session = { token: "admin-token", accountId: "ADMIN1", deviceId: "device", name: "Admin" };
});

afterEach(() => {
  app?.unmount();
  root?.remove();
  app = undefined;
  vi.unstubAllGlobals();
  reauthenticate.mockReset();
  auth.session = null;
});

describe("readAccountId", () => {
  it("reads the account of a detail page and nothing from the list", () => {
    expect(readAccountId("/admin/accounts")).toBe("");
    expect(readAccountId("/admin/accounts/")).toBe("");
    expect(readAccountId("/admin/accounts/ABC123")).toBe("ABC123");
    expect(readAccountId("/admin/accounts/ABC123/")).toBe("ABC123");
    expect(readAccountId("/admin/accounts/ABC123/devices")).toBe("");
  });
});

describe("formatActivity", () => {
  it("says how long ago, and Never without devices", () => {
    const now = Date.parse("2026-10-18T12:00:00Z");
    expect(formatActivity(null, now)).toBe("Never");
    expect(formatActivity("2026-10-18T11:59:30Z", now)).toBe("Just now");
    expect(formatActivity("2026-10-18T11:55:00Z", now)).toBe("5 minutes ago");
    expect(formatActivity("2026-10-17T11:00:00Z", now)).toBe("yesterday");
  });
});

describe("formatDeviceCount", () => {
  it("counts a suspended account's devices as suspended", () => {
    expect(formatDeviceCount(createAccount("ada@example.com", { deviceCount: 2 }))).toBe("2");
    expect(formatDeviceCount(createAccount("ada@example.com", { deviceCount: 2, state: "suspended" }))).toBe("2 suspended");
    expect(formatDeviceCount(createAccount("ada@example.com", { deviceCount: 0, state: "suspended" }))).toBe("0");
  });
});

describe("AccountsView", () => {
  it("lists the accounts and narrows the list as the admin types", async () => {
    currentPath.value = "/admin/accounts";
    fetchMock = vi.fn<typeof fetch>(async (url) => {
      const query = new URL(String(url), "http://hisar.test").searchParams.get("query") ?? "";
      return answer(200, { accounts: allAccount.filter((account) => account.email.includes(query)) });
    });
    vi.stubGlobal("fetch", fetchMock);
    mount(AccountsView);
    await vi.waitFor(() => expect(root.querySelectorAll("tbody tr")).toHaveLength(3));
    expect(root.querySelector('[data-testid="account-ada@example.com"]')?.textContent).toContain("Member");

    type("query", "bo");
    await vi.waitFor(() => expect(root.querySelectorAll("tbody tr")).toHaveLength(1));
    expect(root.querySelector("tbody tr")?.textContent).toContain("bob@example.com");
    expect(String(fetchMock.mock.calls.at(-1)![0])).toBe("/api/v1/admin/accounts?query=bo");

    type("query", "nobody");
    await vi.waitFor(() => expect(root.textContent).toContain("No account matches the search."));
  });
});

describe("AccountView", () => {
  const ada = createAccount("ada@example.com", { id: "ADA123" });
  let roleAnswer: Response;
  let deleteAnswer: Response;

  function mountAccount(account: AdminAccount): void {
    const detail: AdminAccountDetail = { account, devices: [] };
    fetchMock = vi.fn<typeof fetch>(async (url, init) => {
      const path = String(url);
      if (path === "/api/v1/account") {
        return answer(200, { email: "admin@example.com" });
      }
      if (path.endsWith("/role")) {
        return roleAnswer;
      }
      if (path.endsWith("/delete")) {
        return deleteAnswer;
      }
      return init?.method === undefined || init.method === "GET" ? answer(200, detail) : answer(204);
    });
    vi.stubGlobal("fetch", fetchMock);
    mount(AccountView, { accountId: account.id });
  }

  it("disables the role dialog while the master password check runs and shows the server's refusal", async () => {
    roleAnswer = answer(409, { error: "The Hisar server needs at least one admin." });
    mountAccount(ada);
    await vi.waitFor(() => findButton("Change role…"));
    findButton("Change role…").click();
    await vi.waitFor(() => findButton("Change role"));
    type("secretKey", "H1-ADMIN1-SECRET");
    type("password", "admin master password");

    const proof = Promise.withResolvers<ReauthenticationProof>();
    reauthenticate.mockReturnValue(proof.promise);
    root.querySelector<HTMLFormElement>("form[aria-label='Change role']")!.dispatchEvent(new Event("submit"));
    await vi.waitFor(() => expect(findButton("Checking master password…").disabled).toBe(true));
    expect(root.querySelector<HTMLInputElement>("input[name=password]")!.disabled).toBe(true);
    expect(root.querySelector<HTMLInputElement>("input[name=secretKey]")!.disabled).toBe(true);
    expect(findButton("Cancel").disabled).toBe(true);
    expect(reauthenticate).toHaveBeenCalledWith("admin-token", "admin@example.com", "H1-ADMIN1-SECRET", "admin master password");

    proof.resolve({ reauthenticationId: "proof", ke3: "a2Uz" });
    await vi.waitFor(() => expect(root.querySelector("[role=dialog] [role=alert]")?.textContent).toBe("The Hisar server needs at least one admin."));
    expect(findButton("Change role").disabled).toBe(false);
    const [, roleRequest] = fetchMock.mock.calls.find(([url]) => String(url).endsWith("/role"))! as [string, RequestInit];
    expect(JSON.parse(String(roleRequest.body))).toEqual({ role: "admin", proof: { reauthenticationId: "proof", ke3: "a2Uz" } });
  });

  it("asks for the account's email before Delete account and shows a wrong master password", async () => {
    deleteAnswer = answer(204);
    mountAccount(ada);
    await vi.waitFor(() => findButton("Delete account…"));
    findButton("Delete account…").click();
    await vi.waitFor(() => findButton("Delete account"));
    type("secretKey", "H1-ADMIN1-SECRET");
    type("password", "wrong password");
    type("confirmEmail", "someone@example.com");
    await vi.waitFor(() => expect(findButton("Delete account").disabled).toBe(true));
    type("confirmEmail", "Ada@Example.com");
    await vi.waitFor(() => expect(findButton("Delete account").disabled).toBe(false));

    reauthenticate.mockRejectedValue(new Error("incorrect email, master password or Secret Key"));
    root.querySelector<HTMLFormElement>("form[aria-label='Delete account']")!.dispatchEvent(new Event("submit"));
    await vi.waitFor(() => expect(root.querySelector("[role=dialog] [role=alert]")?.textContent).toBe("incorrect email, master password or Secret Key"));
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith("/delete"))).toBe(false);
  });

  it("offers no suspension, role change or deletion of the admin's own account", async () => {
    mountAccount(createAccount("admin@example.com", { id: "ADMIN1", role: "admin" }));
    await vi.waitFor(() => findButton("Sign out everywhere…"));
    for (const label of ["Suspend…", "Change role…", "Delete account…"]) {
      expect(() => findButton(label)).toThrow();
    }
    expect(root.textContent).toContain("Use Settings → Account in the Hisar app for your own account.");
  });

  it("suspends after the dialog and shows Reactivate", async () => {
    let state: AdminAccount["state"] = "active";
    fetchMock = vi.fn<typeof fetch>(async (url, init) => {
      if (String(url).endsWith("/suspend")) {
        state = "suspended";
        return answer(204);
      }
      return init?.method && init.method !== "GET" ? answer(204) : answer(200, { account: { ...ada, state }, devices: [] });
    });
    vi.stubGlobal("fetch", fetchMock);
    mount(AccountView, { accountId: ada.id });
    await vi.waitFor(() => findButton("Suspend…"));
    findButton("Suspend…").click();
    await vi.waitFor(() => findButton("Suspend"));
    findButton("Suspend").click();
    await vi.waitFor(() => expect(root.querySelector("[data-testid=state]")?.textContent?.trim()).toBe("Suspended"));
    expect(findButton("Reactivate").disabled).toBe(false);
  });

  it("shows a suspended account's devices as suspended, with Sign out to remove them", async () => {
    const laptop: AdminDevice = { id: "laptop", name: "Laptop", platform: "macOS", createdAt: "2026-10-01T12:00:00Z", lastSeenAt: "2026-10-18T09:00:00Z", revokedAt: null };
    fetchMock = vi.fn<typeof fetch>(async () => answer(200, { account: { ...ada, state: "suspended" }, devices: [laptop] }));
    vi.stubGlobal("fetch", fetchMock);
    mount(AccountView, { accountId: ada.id });
    await vi.waitFor(() => expect(root.querySelector("[data-testid=device-laptop]")?.textContent).toContain("Suspended · last activity"));
    expect(root.querySelector("[data-testid=device-laptop]")?.textContent).not.toContain("Active");
    expect(findButton("Sign out").disabled).toBe(false);
  });
});
